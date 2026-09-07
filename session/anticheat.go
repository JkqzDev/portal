package session

import (
	"github.com/Weekom-UHC/anticheat-go/check"
	"github.com/Weekom-UHC/anticheat-go/player"
	"github.com/Weekom-UHC/anticheat-go/utils"
	dfevent "github.com/df-mc/dragonfly/server/event"
	"github.com/sandertv/gophertunnel/minecraft/text"
)

type anticheatHandler struct {
	player.NopHandler
	s *Session
}

func (h anticheatHandler) HandleFlag(_ *dfevent.Context[struct{}], c check.Check, params map[string]any, _ *bool) {
	name, variant := c.Name()
	h.s.log.Infof(
		"%s failed %s%s (%.1f/%.1f) %s",
		h.s.conn.IdentityData().DisplayName,
		name, variant,
		c.Violations(), c.MaxViolations(),
		utils.PrettyParameters(params, true),
	)
}

func (h anticheatHandler) HandlePunishment(_ *dfevent.Context[struct{}], c check.Check, message *string) {
	name, variant := c.Name()
	*message = text.Colourf("<red>Kicked by anticheat</red> <grey>(%s%s)</grey>", name, variant)
	h.s.log.Infof("%s was kicked by the anticheat for %s%s", h.s.conn.IdentityData().DisplayName, name, variant)
}
