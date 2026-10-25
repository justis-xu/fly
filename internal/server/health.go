package server

import (
	"fly/internal/checks"
	"fly/internal/core"
)

func (s *Server) nodeHealth() any {
	p, err := core.SelectedProxy()
	if err != nil {
		return nil
	}
	result := map[string]any{"node": p.Name(), "status": "untested"}
	state, ok := p.ExtraDelayHistories()[checks.LatencyURL]
	if !ok || len(state.History) == 0 {
		return result
	}
	last := state.History[len(state.History)-1]
	result["at"] = last.Time
	result["delay"] = last.Delay
	result["status"] = "failed"
	if state.Alive && last.Delay > 0 {
		result["status"] = "ok"
	}
	return result
}
