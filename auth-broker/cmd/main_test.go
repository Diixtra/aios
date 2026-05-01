package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diixtra/aios/auth-broker/internal/server"
)

func TestHealthz(t *testing.T) {
	srv := server.New(server.Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}
