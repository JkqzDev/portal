package session

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Weekom-UHC/anticheat-go/player"
	"github.com/akmalfairuz/legacy-version/legacyver"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/paroxity/portal/event"
	"github.com/paroxity/portal/internal"
	"github.com/paroxity/portal/metrics"
	"github.com/paroxity/portal/server"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/scylladb/go-set/b16set"
	"github.com/scylladb/go-set/i32set"
	"github.com/scylladb/go-set/i64set"
	"github.com/scylladb/go-set/strset"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

// Session stores the data for an active session on the proxy.
type Session struct {
	*translator

	log          internal.Logger
	conn         *minecraft.Conn
	store        *Store
	bus          *event.Bus
	loadBalancer LoadBalancer

	ac *player.Player

	hMutex sync.RWMutex
	// h holds the current handler of the session.
	h Handler

	loginMu        sync.RWMutex
	serverMu       sync.RWMutex
	transferMu     sync.Mutex
	server         *server.Server
	serverConn     *minecraft.Conn
	tempServerConn *minecraft.Conn
	transferDone   func(error)

	entities    *i64set.Set
	playerList  *b16set.Set
	effects     *i32set.Set
	bossBars    *i64set.Set
	scoreboards *strset.Set

	uuid uuid.UUID

	transferring atomic.Bool
	postTransfer atomic.Bool
	dead         atomic.Bool
	closing      atomic.Bool
	once         sync.Once

	dimensionAck chan struct{}
}

// New creates a new Session with the provided connection. bus may be nil, in which case no events are
// published for the session's lifecycle.
func New(conn *minecraft.Conn, store *Store, loadBalancer LoadBalancer, log internal.Logger, bus *event.Bus, antiCheatDisabled bool) (s *Session, err error) {
	s = &Session{
		log:          log,
		conn:         conn,
		store:        store,
		bus:          bus,
		loadBalancer: loadBalancer,

		entities:    i64set.New(),
		playerList:  b16set.New(),
		effects:     i32set.New(),
		bossBars:    i64set.New(),
		scoreboards: strset.New(),

		h:    NopHandler{},
		uuid: uuid.MustParse(conn.IdentityData().Identity),

		dimensionAck: make(chan struct{}, 1),
	}

	store.Store(s)
	defer func() {
		if err != nil {
			store.Delete(s.UUID())
		}
	}()

	srv := loadBalancer.FindServer(s)
	if srv == nil {
		return s, errors.New("load balancer did not return a server for the player to join")
	}
	srv.IncrementPlayerCount()
	s.server = srv

	if store.PlayerConnecting != nil {
		store.PlayerConnecting(srv.Name(), conn.IdentityData().DisplayName, remoteIP(conn.RemoteAddr()))
	}

	s.loginMu.Lock()
	go func() {
		defer s.loginMu.Unlock()
		srvConn, err := s.dial(srv)
		if err != nil {
			log.Errorf("failed to dial server %s: %v (unwrapped: %+v)", srv.Address(), err, errors.Unwrap(err))
			return
		}

		s.serverConn = srvConn
		if err = s.login(); err != nil {
			_ = srvConn.Close()
			log.Errorf("failed to login to server %s: %v", srv.Address(), err)
			return
		}
		log.Infof("%s has been connected to server %s", conn.IdentityData().DisplayName, srv.Name())
		if s.bus != nil {
			s.bus.Publish(event.TopicPlayerJoin, event.PlayerPayload{UUID: s.uuid, Name: conn.IdentityData().DisplayName})
		}

		s.translator = newTranslator(srvConn.GameData())
		if !antiCheatDisabled {
			s.ac = player.NewPlayer(anticheatLogger(log), s.conn, s.serverConn)
			s.ac.Handle(anticheatHandler{s: s})
		}
		handlePackets(s)
	}()
	return s, nil
}

// dial dials a new connection to the provided server. It then returns the connection between the proxy and
// that server, along with any error that may have occurred.
func (s *Session) dial(srv *server.Server) (*minecraft.Conn, error) {
	i := s.conn.IdentityData()
	c := s.conn.ClientData()
	c.PlatformOnlineID = ""
	c.PlatformOfflineID = ""
	c.PlayFabID = ""
	c.ThirdPartyName = i.DisplayName

	return minecraft.Dialer{
		ClientData:          c,
		IdentityData:        i,
		EnableLegacyAuth:    false,
		KeepXBLIdentityData: true,
	}.Dial("raknet", srv.Address())
}

// login performs the initial login sequence for the session.
func (s *Session) login() error {
	var g sync.WaitGroup
	g.Add(2)
	var clientErr, serverErr error
	go func() {
		defer g.Done()
		clientErr = s.conn.StartGameTimeout(s.serverConn.GameData(), time.Minute)
	}()
	go func() {
		defer g.Done()
		serverErr = s.serverConn.DoSpawnTimeout(time.Minute)
	}()
	g.Wait()
	return errors.Join(clientErr, serverErr)
}

// waitForLogin uses the login mutex to wait for the login to complete. If the player is still logging in, loginMu will
// be locked causing this method to block until the login is complete.
func (s *Session) waitForLogin() {
	s.loginMu.RLock()
	s.loginMu.RUnlock()
}

// Conn returns the active connection for the session.
func (s *Session) Conn() *minecraft.Conn {
	s.waitForLogin()
	return s.conn
}

// Server returns the server the session is currently connected to.
func (s *Session) Server() *server.Server {
	s.waitForLogin()
	s.serverMu.RLock()
	defer s.serverMu.RUnlock()
	return s.server
}

// ServerConn returns the connection for the session's current server.
func (s *Session) ServerConn() *minecraft.Conn {
	s.waitForLogin()
	s.serverMu.RLock()
	defer s.serverMu.RUnlock()
	return s.serverConn
}

// UUID returns the UUID from the session's connection.
func (s *Session) UUID() uuid.UUID {
	return s.uuid
}

// Handle sets the handler for the current session which can be used to handle different events from the
// session. If the handler is nil, a NopHandler is used instead.
func (s *Session) Handle(h Handler) {
	s.hMutex.Lock()
	defer s.hMutex.Unlock()

	if h == nil {
		h = NopHandler{}
	}
	s.h = h
}

// Transfer transfers the session to the provided server, returning any error that may have occurred during
// the initial transfer.
func (s *Session) Transfer(srv *server.Server) (err error) {
	s.waitForLogin()
	if !s.transferring.CAS(false, true) {
		return errors.New("already being transferred")
	}
	s.postTransfer.Store(false)

	fromName := s.Server().Name()
	s.log.Infof("%s is being transferred from %s to %s", s.conn.IdentityData().DisplayName, fromName, srv.Name())

	start := time.Now()
	publishTransfer := func(transferErr error) {
		metrics.Default.RecordTransfer(transferErr == nil, time.Since(start))
		if s.bus != nil {
			s.bus.Publish(event.TopicTransfer, event.TransferPayload{
				PlayerName: s.conn.IdentityData().DisplayName,
				FromServer: fromName,
				ToServer:   srv.Name(),
				Err:        transferErr,
			})
		}
	}
	s.transferMu.Lock()
	s.transferDone = publishTransfer
	s.transferMu.Unlock()

	ctx := event.C()
	s.handler().HandleTransfer(ctx, srv)

	ctx.Continue(func() {
		// If the player is dead, force-respawn them before transferring.
		// Without this, the dimension trick fails and the player gets stuck.
		if s.dead.CAS(true, false) {
			_ = s.conn.WritePacket(&packet.Respawn{
				Position:        s.conn.GameData().PlayerPosition,
				State:           packet.RespawnStateReadyToSpawn,
				EntityRuntimeID: s.originalRuntimeID,
			})
		}

		// Notify the target server to disconnect any stale session for this player.
		// This prevents the "already logged in" error when the old Raknet session
		// hasn't been cleaned up yet on servers like GeyserMC/Spigot.
		if s.store.PreTransfer != nil {
			s.store.PreTransfer(srv.Name(), s.conn.IdentityData().DisplayName)
			time.Sleep(1 * time.Second)
		}

		if s.store.PlayerConnecting != nil {
			s.store.PlayerConnecting(srv.Name(), s.conn.IdentityData().DisplayName, remoteIP(s.conn.RemoteAddr()))
		}

		var conn *minecraft.Conn
		conn, err = s.dial(srv)
		if err != nil {
			// If the server still thinks the player is logged in, retry once after a longer delay
			// to allow the Spigot server to fully clean up the kicked session.
			if strings.Contains(err.Error(), "already logged in") {
				s.log.Debugf("retrying transfer to %s for %s after 'already logged in' error", srv.Name(), s.conn.IdentityData().DisplayName)
				time.Sleep(3 * time.Second)
				conn, err = s.dial(srv)
			}
			if err != nil {
				s.log.Errorf("transfer failed: could not dial %s: %v", srv.Name(), err)
				s.setTransferring(false)
				s.completeTransfer(err)
				return
			}
		}
		if err = conn.DoSpawnTimeout(time.Minute); err != nil {
			_ = conn.Close()
			s.log.Errorf("transfer failed: spawn timeout on %s: %v", srv.Name(), err)
			s.setTransferring(false)
			s.completeTransfer(err)
			return
		}

		gameData := conn.GameData()

		s.serverMu.Lock()
		currentDimension := s.serverConn.GameData().Dimension
		oldServerConn := s.serverConn
		s.serverMu.Unlock()

		if currentDimension == gameData.Dimension {
			select {
			case <-s.dimensionAck:
			default:
			}
			proxyDimension := selectProxyDimension(currentDimension, gameData.Dimension)
			decoyPos := gameData.PlayerPosition.Add(mgl32.Vec3{2000, 0, 2000})
			s.changeDimension(proxyDimension, decoyPos)
			select {
			case <-s.dimensionAck:
			case <-time.After(250 * time.Millisecond):
			}
		}

		s.serverMu.Lock()
		s.serverConn = conn
		s.updateTranslatorData(gameData)
		s.serverMu.Unlock()

		_ = oldServerConn.WritePacket(&packet.Disconnect{
			Message: "Server transfer",
		})
		_ = oldServerConn.Close()

		s.changeDimension(gameData.Dimension, gameData.PlayerPosition)

		_ = conn.WritePacket(&packet.SetLocalPlayerAsInitialised{EntityRuntimeID: gameData.EntityRuntimeID})

		var w sync.WaitGroup
		w.Add(2)
		go func() {
			s.clearEntities()
			s.clearEffects()
			w.Done()
		}()
		go func() {
			s.clearPlayerList()
			s.clearBossBars()
			s.clearScoreboard()
			w.Done()
		}()

		_ = s.conn.WritePacket(&packet.MovePlayer{
			EntityRuntimeID: s.originalRuntimeID,
			Position:        gameData.PlayerPosition,
			Pitch:           gameData.Pitch,
			Yaw:             gameData.Yaw,
			Mode:            packet.MoveModeReset,
		})
		_ = s.conn.WritePacket(&packet.LevelEvent{EventType: packet.LevelEventStopRaining, EventData: 10000})
		_ = s.conn.WritePacket(&packet.LevelEvent{EventType: packet.LevelEventStopThunderstorm})
		_ = s.conn.WritePacket(&packet.SetDifficulty{Difficulty: uint32(gameData.Difficulty)})
		_ = s.conn.WritePacket(&packet.GameRulesChanged{GameRules: gameData.GameRules})
		_ = s.conn.WritePacket(&packet.SetPlayerGameType{GameType: gameData.PlayerGameMode})
		_ = s.conn.WritePacket(&packet.NetworkChunkPublisherUpdate{
			Position: protocol.BlockPos{int32(gameData.PlayerPosition.X()), int32(gameData.PlayerPosition.Y()), int32(gameData.PlayerPosition.Z())},
			Radius:   uint32(gameData.ChunkRadius) << 4,
		})
		radius := gameData.ChunkRadius
		if radius < 1 {
			radius = 1
		}
		_ = conn.WritePacket(&packet.RequestChunkRadius{ChunkRadius: radius, MaxChunkRadius: uint8(radius)})

		if s.dead.CAS(true, false) {
			_ = s.conn.WritePacket(&packet.Respawn{
				Position:        gameData.PlayerPosition,
				State:           packet.RespawnStateReadyToSpawn,
				EntityRuntimeID: s.originalRuntimeID,
			})
		}

		w.Wait()
		_ = s.conn.Flush()

		if s.ac != nil {
			s.ac.SetServerConn(conn)
			s.ac.SetRuntimeID(gameData.EntityRuntimeID)
			s.ac.SetUniqueID(gameData.EntityUniqueID)
		}

		s.serverMu.Lock()
		s.server.DecrementPlayerCount()
		s.server = srv
		s.server.IncrementPlayerCount()
		s.serverMu.Unlock()

		s.setTransferring(false)
		s.postTransfer.Store(true)
		go func() {
			time.Sleep(30 * time.Second)
			s.postTransfer.Store(false)
		}()
		s.log.Infof("%s finished transferring to %s", s.conn.IdentityData().DisplayName, srv.Name())
		s.completeTransfer(nil)
	})

	ctx.Stop(func() {
		s.setTransferring(false)
		s.completeTransfer(errors.New("transfer cancelled"))
	})

	return
}

func (s *Session) fallbackTransfer() bool {
	if s.closing.Load() {
		return false
	}
	fallback := s.loadBalancer.FindServer(s)
	if fallback == nil || fallback == s.Server() {
		return false
	}
	if err := s.Transfer(fallback); err != nil {
		s.log.Errorf("fallback transfer to %s failed for %s: %v", fallback.Name(), s.conn.IdentityData().DisplayName, err)
		return false
	}
	return true
}

func (s *Session) completeTransfer(err error) {
	s.transferMu.Lock()
	done := s.transferDone
	s.transferDone = nil
	s.transferMu.Unlock()
	if done != nil {
		done(err)
	}
}

// Transferring returns if the session is currently transferring to a different server or not.
func (s *Session) Transferring() bool {
	return s.transferring.Load()
}

// setTransferring sets if the session is transferring to a different server.
func (s *Session) setTransferring(v bool) {
	s.transferring.Store(v)
}

// handler() returns the handler connected to the session.
func (s *Session) handler() Handler {
	s.hMutex.RLock()
	defer s.hMutex.RUnlock()
	return s.h
}

func anticheatLogger(l internal.Logger) *logrus.Logger {
	if lg, ok := l.(*logrus.Logger); ok {
		return lg
	}
	return logrus.New()
}

// Close closes the session and any linked connections/counters.
func (s *Session) Close() {
	s.closing.Store(true)
	s.once.Do(func() {
		if s.transferring.CAS(true, false) {
			s.postTransfer.Store(false)
			s.completeTransfer(errors.New("session closed during transfer"))
		}
		if s.ac != nil {
			_ = s.ac.Close()
		}
		s.handler().HandleQuit()
		s.Handle(NopHandler{})

		if s.bus != nil {
			s.bus.Publish(event.TopicPlayerQuit, event.PlayerPayload{UUID: s.uuid, Name: s.conn.IdentityData().DisplayName})
		}

		s.store.Delete(s.UUID())

		legacyver.ClearConnState(s.conn)
		_ = s.conn.Close()
		if s.serverConn != nil {
			_ = s.serverConn.Close()
		}
		if s.tempServerConn != nil {
			_ = s.tempServerConn.Close()
		}

		if s.server != nil {
			s.server.DecrementPlayerCount()
		}
	})
}

// Disconnect disconnects the session from the proxy and shows them the provided message. If the message is empty, the
// player will be immediately sent to the server list instead of seeing the disconnect screen.
func (s *Session) Disconnect(message string) {
	_ = s.conn.WritePacket(&packet.Disconnect{
		HideDisconnectionScreen: message == "",
		Message:                 message,
	})
	s.Close()
}

// clearEntities flushes the entities map and despawns the entities for the client.
func (s *Session) clearEntities() {
	s.entities.Each(func(id int64) bool {
		_ = s.conn.WritePacket(&packet.RemoveActor{EntityUniqueID: id})
		return true
	})

	s.entities.Clear()
}

// clearPlayerList flushes the playerList map and removes all the entries for the client.
func (s *Session) clearPlayerList() {
	var entries = make([]protocol.PlayerListEntry, 0, s.playerList.Size())
	s.playerList.Each(func(uid [16]byte) bool {
		entries = append(entries, protocol.PlayerListEntry{ActionType: protocol.PlayerListActionRemove, UUID: uid})
		return true
	})

	_ = s.conn.WritePacket(&packet.PlayerList{Entries: entries})

	s.playerList.Clear()
}

// clearEffects flushes the effects map and removes all the effects for the client.
func (s *Session) clearEffects() {
	s.effects.Each(func(i int32) bool {
		_ = s.conn.WritePacket(&packet.MobEffect{
			EntityRuntimeID: s.originalRuntimeID,
			Operation:       packet.MobEffectRemove,
			EffectType:      i,
		})
		return true
	})

	s.effects.Clear()
}

// clearBossBars clears all the boss bars currently visible the client.
func (s *Session) clearBossBars() {
	s.bossBars.Each(func(b int64) bool {
		_ = s.conn.WritePacket(&packet.BossEvent{
			BossEntityUniqueID: b,
			EventType:          packet.BossEventHide,
		})
		return true
	})

	s.bossBars.Clear()
}

// clearScoreboard clears the current scoreboard visible by the client.
func (s *Session) clearScoreboard() {
	s.scoreboards.Each(func(sb string) bool {
		_ = s.conn.WritePacket(&packet.RemoveObjective{ObjectiveName: sb})
		return true
	})

	s.scoreboards.Clear()
}

func (s *Session) changeDimension(dimension int32, pos mgl32.Vec3) {
	_ = s.conn.WritePacket(&packet.ChangeDimension{
		Dimension: dimension,
		Position:  pos,
	})
	_ = s.conn.WritePacket(&packet.StopSound{StopAll: true})
	_ = s.conn.WritePacket(&packet.PlayerAction{EntityRuntimeID: s.originalRuntimeID, ActionType: protocol.PlayerActionDimensionChangeDone})
}

func selectProxyDimension(source, target int32) int32 {
	for _, dimension := range []int32{packet.DimensionOverworld, packet.DimensionNether, packet.DimensionEnd} {
		if dimension != source && dimension != target {
			return dimension
		}
	}
	return packet.DimensionOverworld
}

// remoteIP returns just the IP portion of a net.Addr, dropping the port. If it can't be split into host and
// port (unexpected address format), the address's string form is returned as-is.
func remoteIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
