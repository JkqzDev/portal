package session

import (
	"github.com/Weekom-UHC/anticheat-go/check"
	"github.com/Weekom-UHC/anticheat-go/player"
	"github.com/Weekom-UHC/anticheat-go/utils"
	dfevent "github.com/df-mc/dragonfly/server/event"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
)

type anticheatHandler struct {
	player.NopHandler
	s *Session
}

func (h anticheatHandler) HandleFlag(_ *dfevent.Context[struct{}], c check.Check, params map[string]any, _ *bool) {
	name, variant := c.Name()
	name += variant
	player := h.s.conn.IdentityData().DisplayName

	h.s.log.Infof("%s failed %s (%.1f/%.1f) %s", player, name, c.Violations(), c.MaxViolations(), utils.PrettyParameters(params, true))

	h.broadcast(text.Colourf(
		"<grey>[</grey><red>AntiCheat</red><grey>]</grey> <yellow>%s</yellow> <grey>is suspected of using</grey> <white>%s</white> <grey>(%.1f/%.1f)</grey>",
		player, name, c.Violations(), c.MaxViolations(),
	))
}

func (h anticheatHandler) HandlePunishment(_ *dfevent.Context[struct{}], c check.Check, message *string) {
	name, variant := c.Name()
	name += variant
	player := h.s.conn.IdentityData().DisplayName

	*message = text.Colourf("<red>Kicked by anticheat</red> <grey>(%s)</grey>", name)
	h.s.log.Infof("%s was kicked by the anticheat for %s", player, name)

	h.broadcast(text.Colourf(
		"<grey>[</grey><red>AntiCheat</red><grey>]</grey> <yellow>%s</yellow> <red>was removed for using</red> <white>%s</white>",
		player, name,
	))
}

func (h anticheatHandler) broadcast(message string) {
	for _, s := range h.s.store.All() {
		_ = s.conn.WritePacket(&packet.Text{TextType: packet.TextTypeRaw, Message: message})
	}
}
