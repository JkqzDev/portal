package session

import (
	"strings"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
)

// handleCommandRequest intercepts commands portal itself handles, without forwarding them to the backend
// server. It returns true if the command was handled.
func handleCommandRequest(s *Session, pk *packet.CommandRequest) bool {
	fields := strings.Fields(pk.CommandLine)
	if len(fields) == 0 {
		return false
	}

	switch strings.ToLower(fields[0]) {
	case "/protocol":
		s.handleProtocolCommand(fields[1:])
		return true
	}

	return false
}

func (s *Session) handleProtocolCommand(args []string) {
	if len(args) < 1 {
		s.sendMessage(text.Colourf("<red>Usage: /protocol <player></red>"))
		return
	}

	target, ok := s.store.LoadFromName(args[0])
	if !ok {
		s.sendMessage(text.Colourf("<red>Player not found.</red>"))
		return
	}

	proto := target.Conn().Proto()
	s.sendMessage(text.Colourf(
		"<purple>%s</purple> <grey>is connected on version</grey> <purple>v%s</purple> <grey>(%d)</grey>",
		target.Conn().IdentityData().DisplayName, proto.Ver(), proto.ID(),
	))
}

func (s *Session) sendMessage(message string) {
	_ = s.conn.WritePacket(&packet.Text{TextType: packet.TextTypeRaw, Message: message})
}
