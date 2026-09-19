package packet

import "github.com/sandertv/gophertunnel/minecraft/protocol"

// PlayerAddress is sent by the proxy to a server right after a player finishes connecting to it, carrying
// the player's real remote address as seen by the proxy. Every player reaches the server through the same
// proxy connection, so without this the server only ever sees the proxy's own address - which breaks any
// IP-based logic on the server (bans, anti-VPN, logging), and means a single flagged connection can end up
// blocking the proxy's address for every player behind it instead of just the player responsible.
type PlayerAddress struct {
	// PlayerName is the name of the player that connected.
	PlayerName string
	// Address is the player's real remote address (IP, without port) as seen by the proxy.
	Address string
}

// ID ...
func (*PlayerAddress) ID() uint16 {
	return IDPlayerAddress
}

// Marshal ...
func (pk *PlayerAddress) Marshal(w *protocol.Writer) {
	w.String(&pk.PlayerName)
	w.String(&pk.Address)
}

// Unmarshal ...
func (pk *PlayerAddress) Unmarshal(r *protocol.Reader) {
	r.String(&pk.PlayerName)
	r.String(&pk.Address)
}
