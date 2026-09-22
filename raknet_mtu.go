package portal

import (
	"context"
	"log/slog"
	"net"

	"github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft"
)

// raknetMaxMTU caps the MTU used during the RakNet handshake and for the connection afterwards. Portal
// proxies every packet across two separate RakNet connections (client<->Portal, Portal<->backend), so a
// large packet like LevelChunk gets fragmented and reassembled twice instead of once. Capping the MTU here
// keeps that fragmentation consistent and avoids relying on gophertunnel's default (1492) auto-negotiation,
// which was written for a single direct hop.
const raknetMaxMTU = 1400

// raknetNetwork is a drop-in replacement for gophertunnel's built-in "raknet" network that additionally
// caps the MTU used for both the listener (client-facing) and the dialer (backend-facing) side.
type raknetNetwork struct {
	l *slog.Logger
}

func (r raknetNetwork) DialContext(ctx context.Context, address string) (net.Conn, error) {
	return raknet.Dialer{
		ErrorLog:           r.l.With("net origin", "raknet"),
		MaxMTU:             raknetMaxMTU,
		MaxTransientErrors: -1,
	}.DialContext(ctx, address)
}

func (r raknetNetwork) PingContext(ctx context.Context, address string) ([]byte, error) {
	return raknet.Dialer{
		ErrorLog: r.l.With("net origin", "raknet"),
		MaxMTU:   raknetMaxMTU,
	}.PingContext(ctx, address)
}

func (r raknetNetwork) Listen(address string) (minecraft.NetworkListener, error) {
	return raknet.ListenConfig{ErrorLog: r.l.With("net origin", "raknet"), MaxMTU: raknetMaxMTU}.Listen(address)
}

// init overrides the "raknet" network registered by gophertunnel's own init. Dependency package inits run
// before this one, so this registration wins.
func init() {
	minecraft.RegisterNetwork("raknet", func(l *slog.Logger) minecraft.Network {
		return raknetNetwork{l: l}
	})
}
