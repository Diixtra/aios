package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Diixtra/aios/auth-broker/internal/auth"
	"github.com/Diixtra/aios/auth-broker/internal/config"
	"github.com/Diixtra/aios/auth-broker/internal/lease"
	"github.com/Diixtra/aios/auth-broker/internal/notify"
	"github.com/Diixtra/aios/auth-broker/internal/scheduler"
	"github.com/Diixtra/aios/auth-broker/internal/server"
	"github.com/Diixtra/aios/auth-broker/internal/store"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	st := store.New(filepath.Join(cfg.PiDir, "auth.json"))
	leaseMgr := lease.New(cfg.LeaseCap, cfg.LeaseTTL)
	validator := auth.NewValidator(cfg.PiBinary, cfg.PiDir)
	refresher := auth.NewRefresher(cfg.PiBinary, cfg.PiDir)

	slack := notify.NewRealSlack(cfg.SlackToken)
	notifier := notify.NewNotifier(slack, cfg.SlackDMUserID, "http://"+cfg.ListenAddr)

	sm := auth.NewMachine()
	orch := auth.NewOrchestrator(sm, validator, notifier)
	orch.SetBundleAge(bundleAgeFn(filepath.Join(cfg.PiDir, "auth.json")))

	apiSrv := server.New(server.Config{Lease: leaseMgr, Store: st, Orchestrator: orch})
	bundleHandler := server.NewBundleHandler(st, func() {
		_ = orch.OnBundleUploaded(context.Background())
	})

	mux := http.NewServeMux()
	mux.Handle("/", apiSrv.Handler())
	mux.Handle("POST /v1/auth/bundle", server.AdminMiddleware(cfg.AdminToken, bundleHandler))

	// Periodic refresh + revalidation.
	refresh := scheduler.New(cfg.RefreshInterval, func(ctx context.Context) error {
		_ = refresher.Refresh(ctx)
		return orch.Tick(ctx)
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Best-effort: wire k8s in-cluster client for SA-token TokenReview if available.
	// Outside the cluster (local dev) the broker boots without and rejects SA-authed
	// endpoints; admin endpoints still work.
	if rc, err := rest.InClusterConfig(); err == nil {
		if k8s, err := kubernetes.NewForConfig(rc); err == nil {
			_ = k8s // wired into middleware in a future task; left no-op for now
			slog.Info("kubernetes client initialised")
		}
	}

	go refresh.Run(ctx)

	hsrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, sc := context.WithTimeout(context.Background(), 5*time.Second)
		defer sc()
		_ = hsrv.Shutdown(shutdownCtx)
	}()

	slog.Info("auth-broker listening", "addr", cfg.ListenAddr, "lease_cap", cfg.LeaseCap)
	if err := hsrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

// bundleAgeFn returns a closure satisfying Orchestrator.bundleAge: reads the
// auth.json mtime and returns days-since-modified plus a presence flag.
func bundleAgeFn(path string) func() (int, bool) {
	return func() (int, bool) {
		info, err := os.Stat(path)
		if err != nil {
			return 0, false
		}
		days := int(time.Since(info.ModTime()).Hours() / 24)
		return days, true
	}
}
