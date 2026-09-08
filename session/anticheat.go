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

	info := utils.PrettyParameters(params, true)

	h.s.log.Infof("%s failed %s %s", player, name, info)

	h.broadcast(text.Colourf(
		"<grey>[</grey><red>AntiCheat</red><grey>]</grey> <yellow>%s</yellow> <grey>is suspected of using</grey> <white>%s</white> <dark-grey>%s</dark-grey>",
		player, name, info,
	))
}

func (h anticheatHandler) broadcast(message string) {
	for _, s := range h.s.store.All() {
		_ = s.conn.WritePacket(&packet.Text{TextType: packet.TextTypeRaw, Message: message})
	}
}
