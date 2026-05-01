package server

import (
	"context"
	"net/http"

	"github.com/Diixtra/aios/auth-broker/internal/lease"
	"github.com/Diixtra/aios/auth-broker/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type orchestrator interface {
	OnBundleUploaded(ctx context.Context) error
}

type Config struct {
	Lease        *lease.Manager
	Store        *store.Store
	Orchestrator orchestrator
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.mux.Handle("GET /metrics", promhttp.Handler())
	s.mux.HandleFunc("POST /v1/leases/acquire", s.acquireLease)
	s.mux.HandleFunc("POST /v1/leases/release", s.releaseLease)
	s.mux.HandleFunc("GET /v1/auth/bundle", s.authBundle)
	s.mux.HandleFunc("POST /v1/auth/bundle/post-run", s.postRunBundle)
	s.mux.HandleFunc("POST /v1/admin/revalidate", s.revalidate)
	// POST /v1/auth/bundle (bootstrap upload) is registered separately by main.go
	// alongside this server, since it is admin-token-guarded while the rest
	// of the surface is SA-token-guarded. See cmd/main.go.
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
