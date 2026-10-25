package server

import (
	"encoding/json"
	"fly/internal/config"
	"fly/internal/sub"
	"net/http"
	"strconv"
)

func (s *Server) handleRouting(w http.ResponseWriter, r *http.Request) {
	rules, err := config.LoadRules(s.app.Dir)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	result := map[string]any{"user_rules": rules}
	if host := r.URL.Query().Get("host"); host != "" {
		index, err := config.MatchUserRule(rules, host)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		result["matched_index"] = index
	}
	writeJSON(w, result)
}
func (s *Server) handleSaveRouting(w http.ResponseWriter, r *http.Request) {
	if s.updating.Load() || s.app.Starting() {
		http.Error(w, "正在加载订阅，请稍后保存规则", 409)
		return
	}
	var body struct {
		Rules []config.UserRule `json:"user_rules"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "规则输入无效", 400)
		return
	}
	if err := s.app.SaveUserRules(body.Rules); err != nil {
		http.Error(w, sub.Redact(err.Error()), 400)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) handleProviderRules(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 100
	}
	page, err := s.app.ProviderRules(r.PathValue("name"), r.URL.Query().Get("q"), offset, limit)
	if err != nil {
		http.Error(w, sub.Redact(err.Error()), 400)
		return
	}
	writeJSON(w, page)
}
