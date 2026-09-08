package session

import (
	"encoding/json"

	"github.com/Weekom-UHC/anticheat-go/check"
	"github.com/Weekom-UHC/anticheat-go/player"
	"github.com/Weekom-UHC/anticheat-go/utils"
	dfevent "github.com/df-mc/dragonfly/server/event"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
)

const anticheatNotifyIdentifier = "anticheat:notify"

type anticheatHandler struct {
	player.NopHandler
	s *Session
}

func (h anticheatHandler) HandleFlag(_ *dfevent.Context[struct{}], c check.Check, params map[string]any, _ *bool) {
	name, variant := c.Name()
	player := h.s.conn.IdentityData().DisplayName

	h.s.log.Infof(
		"%s failed %s%s (%.1f/%.1f) %s",
		player, name, variant, c.Violations(), c.MaxViolations(), utils.PrettyParameters(params, true),
	)

	h.notify("flag", name+variant, player, text.Colourf(
		"<grey>[</grey><red>AC</red><grey>]</grey> <yellow>%s</yellow> <grey>failed</grey> <white>%s%s</white> <grey>(%.1f/%.1f)</grey> <dark-grey>%s</dark-grey>",
		player, name, variant, c.Violations(), c.MaxViolations(), utils.PrettyParameters(params, true),
	))
}

func (h anticheatHandler) HandlePunishment(_ *dfevent.Context[struct{}], c check.Check, message *string) {
	name, variant := c.Name()
	player := h.s.conn.IdentityData().DisplayName

	*message = text.Colourf("<red>Kicked by anticheat</red> <grey>(%s%s)</grey>", name, variant)
	h.s.log.Infof("%s was kicked by the anticheat for %s%s", player, name, variant)

	h.notify("punishment", name+variant, player, text.Colourf(
		"<grey>[</grey><red>AC</red><grey>]</grey> <yellow>%s</yellow> <red>was kicked for</red> <white>%s%s</white>",
		player, name, variant,
	))
}

func (h anticheatHandler) notify(kind, check, player, message string) {
	conn := h.s.ServerConn()
	if conn == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"type":    kind,
		"check":   check,
		"player":  player,
		"message": message,
	})
	if err != nil {
		return
	}

	_ = conn.WritePacket(&packet.ScriptMessage{
		Identifier: anticheatNotifyIdentifier,
		Data:       payload,
	})
}
