package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/Diixtra/aios/auth-broker/internal/server"
)

func main() {
	srv := server.New(server.Config{})
	addr := ":8080"
	slog.Info("auth-broker listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
