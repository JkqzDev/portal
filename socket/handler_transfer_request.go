package socket

import (
	"errors"

	"github.com/paroxity/portal/socket/packet"
	"github.com/sandertv/gophertunnel/minecraft"
)

// TransferRequestHandler is responsible for handling the TransferRequest packet sent by servers.
type TransferRequestHandler struct{ requireAuth }

// Handle ...
func (*TransferRequestHandler) Handle(p packet.Packet, srv Server, c *Client) error {
	pk := p.(*packet.TransferRequest)
	response := func(status byte, error string) error {
		return c.WritePacket(&packet.TransferResponse{
			PlayerUUID: pk.PlayerUUID,
			Status:     status,
			Error:      error,
		})
	}

	targetSrv, ok := srv.ServerRegistry().Server(pk.Server)
	if !ok {
		return response(packet.TransferResponseServerNotFound, "server "+pk.Server+" not found")
	}

	s, ok := srv.SessionStore().Load(pk.PlayerUUID)
	if !ok {
		return response(packet.TransferResponsePlayerNotFound, "player not found")
	}

	if s.Server().Address() == targetSrv.Address() {
		return response(packet.TransferResponseAlreadyOnServer, "player is already on "+pk.Server)
	}

	if err := s.Transfer(targetSrv); err != nil {
		// If the target server rejected the connection with a Disconnect packet (e.g. its own
		// whitelist), surface just that message rather than the full dial/receive error chain
		// wrapping it.
		var disconnectErr minecraft.DisconnectError
		if errors.As(err, &disconnectErr) {
			return response(packet.TransferResponseError, disconnectErr.Error())
		}
		return response(packet.TransferResponseError, err.Error())
	}

	return response(packet.TransferResponseSuccess, "")
}
