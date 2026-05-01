package server

import (
	"encoding/json"
	"errors"
	"net/http"
)

type acquireReq struct {
	Holder string `json:"holder"`
}

type acquireResp struct {
	LeaseID   string `json:"lease_id"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) acquireLease(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Lease == nil {
		http.Error(w, "lease manager not configured", http.StatusInternalServerError)
		return
	}
	var req acquireReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Holder == "" {
		http.Error(w, "holder is required", http.StatusBadRequest)
		return
	}
	lease, err := s.cfg.Lease.Acquire(r.Context(), req.Holder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(acquireResp{
		LeaseID:   lease.ID,
		ExpiresAt: lease.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

type releaseReq struct {
	LeaseID string `json:"lease_id"`
}

func (s *Server) releaseLease(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Lease == nil {
		http.Error(w, "lease manager not configured", http.StatusInternalServerError)
		return
	}
	var req releaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.LeaseID == "" {
		http.Error(w, "lease_id is required", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Lease.Release(req.LeaseID); err != nil {
		if errors.Is(err, errUnknownLease) || err.Error() == "lease: unknown id" {
			http.Error(w, "unknown lease_id", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// errUnknownLease is a sentinel placeholder used by the leases handler. We
// keep the lease package's error opaque (a plain errors.New) to avoid leaking
// implementation details across the package boundary; this var preserves the
// errors.Is shape if the lease package upgrades to a typed error later.
var errUnknownLease = errors.New("lease: unknown id")
