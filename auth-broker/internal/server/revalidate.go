package server

import (
	"encoding/json"
	"net/http"
)

type revalidateResp struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

func (s *Server) revalidate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Orchestrator == nil {
		http.Error(w, "orchestrator not configured", http.StatusInternalServerError)
		return
	}
	resp := revalidateResp{State: "Healthy"}
	if err := s.cfg.Orchestrator.OnBundleUploaded(r.Context()); err != nil {
		resp.State = "Expired"
		resp.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
