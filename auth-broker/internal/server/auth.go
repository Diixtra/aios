package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/Diixtra/aios/auth-broker/internal/store"
)

func (s *Server) authBundle(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Store == nil {
		http.Error(w, "store not configured", http.StatusInternalServerError)
		return
	}
	b, err := s.cfg.Store.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "no bundle uploaded yet", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b.Raw)
}

func (s *Server) postRunBundle(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		http.Error(w, "store not configured", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, bundleSizeLimit+1)
	if err := r.ParseMultipartForm(bundleSizeLimit); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "bundle too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "missing bundle field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if !json.Valid(raw) {
		http.Error(w, "bundle must be JSON", http.StatusBadRequest)
		return
	}
	cur, err := s.cfg.Store.Read()
	if err == nil && !isNewer(raw, cur.Raw) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":false,"reason":"older or equal"}`))
		return
	}
	if err := s.cfg.Store.Write(store.Bundle{Raw: raw}); err != nil {
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"accepted":true}`))
}

// isNewer compares the openai-codex.expires field. Returns true if incoming
// expires > stored expires. Falls open (returns true) if either side is
// unparseable to avoid blocking refresh on schema changes; the orchestrator
// runs Validate after each accepted upload anyway.
func isNewer(incoming, stored []byte) bool {
	type bundle struct {
		Codex struct {
			Expires int64 `json:"expires"`
		} `json:"openai-codex"`
	}
	var in, sv bundle
	if err := json.Unmarshal(incoming, &in); err != nil {
		return true
	}
	if err := json.Unmarshal(stored, &sv); err != nil {
		return true
	}
	return in.Codex.Expires > sv.Codex.Expires
}
