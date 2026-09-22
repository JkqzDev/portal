package socket

import (
	"github.com/paroxity/portal/socket/packet"
)

// ServerListRequestHandler is responsible for handling the ServerListRequest packet sent by servers.
type ServerListRequestHandler struct{ requireAuth }

// Handle ...
func (*ServerListRequestHandler) Handle(_ packet.Packet, srv Server, c *Client) error {
	players := map[string][]string{}
	for _, s := range srv.SessionStore().All() {
		name := s.Server().Name()
		players[name] = append(players[name], s.Conn().IdentityData().DisplayName)
	}

	var servers []packet.ServerEntry

	for _, s := range srv.ServerRegistry().Servers() {
		entry := packet.ServerEntry{
			Name:        s.Name(),
			PlayerCount: int64(s.PlayerCount()),
			Players:     players[s.Name()],
		}
		servers = append(servers, entry)
	}

	return c.WritePacket(&packet.ServerListResponse{
		Servers: servers,
	})
}
