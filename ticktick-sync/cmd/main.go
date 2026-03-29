package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Diixtra/aios/ticktick-sync/internal/config"
	"github.com/Diixtra/aios/ticktick-sync/internal/ghclient"
	"github.com/Diixtra/aios/ticktick-sync/internal/state"
	"github.com/Diixtra/aios/ticktick-sync/internal/sync"
	"github.com/Diixtra/aios/ticktick-sync/internal/ticktick"
	"github.com/Diixtra/aios/ticktick-sync/internal/webhook"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting ticktick-sync",
		"repos", cfg.GitHubRepos,
		"project", cfg.TickTickProjectID,
		"interval", cfg.PollInterval,
	)

	ttClient := ticktick.NewDefaultClient(cfg.TickTickAccessToken)
	ghClient := ghclient.NewClient(cfg.GitHubToken)

	var store state.Store
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		slog.Warn("not running in cluster, using memory store", "error", err)
		store = state.NewMemoryStore()
	} else {
		clientset, err := kubernetes.NewForConfig(k8sCfg)
		if err != nil {
			slog.Error("failed to create k8s client", "error", err)
			os.Exit(1)
		}
		cmStore := state.NewConfigMapStore(clientset, cfg.Namespace)
		if err := cmStore.Load(context.Background()); err != nil {
			slog.Error("failed to load state", "error", err)
			os.Exit(1)
		}
		store = cmStore
	}

	engine := sync.NewEngine(ttClient, ghClient, store, cfg.TickTickProjectID, cfg.GitHubRepos)

	// HTTP server: webhook + health
	mux := http.NewServeMux()
	mux.Handle("/webhook/github", webhook.NewHandler([]byte(cfg.GitHubWebhookSecret), engine))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		slog.Info("http server listening", "addr", ":8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Run first poll immediately
	if err := engine.PollTickTick(ctx); err != nil {
		slog.Error("initial poll failed", "error", err)
	}

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := engine.PollTickTick(ctx); err != nil {
				slog.Error("poll cycle failed", "error", err)
			}
		case sig := <-sigCh:
			slog.Info("received signal, shutting down", "signal", sig)
			cancel()
			srv.Shutdown(context.Background())
			if err := store.Flush(context.Background()); err != nil {
				slog.Error("failed to flush state on shutdown", "error", err)
			}
			return
		}
	}
}
