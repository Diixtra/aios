package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gh "github.com/Diixtra/aios/webhook/internal/github"
	k8sclient "github.com/Diixtra/aios/webhook/internal/k8s"
	"github.com/Diixtra/aios/webhook/internal/paperless"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Fatal("GITHUB_WEBHOOK_SECRET environment variable is required")
	}

	namespace := os.Getenv("AIOS_NAMESPACE")
	if namespace == "" {
		namespace = "aios"
	}

	// Create the K8s client for AgentTask CR creation.
	kc, err := k8sclient.NewClient(namespace)
	if err != nil {
		log.Fatalf("failed to create K8s client: %v", err)
	}

	createTask := func(req gh.TaskRequest) error {
		params := k8sclient.TaskParams{
			Repo:        req.Repo,
			IssueNumber: req.IssueNumber,
			Title:       req.Title,
			Body:        req.Body,
			Labels:      req.Labels,
		}
		if err := kc.CreateAgentTask(context.Background(), params); err != nil {
			log.Printf("failed to create AgentTask: %v", err)
			return err
		}
		log.Printf("created AgentTask: repo=%s issue=#%d", req.Repo, req.IssueNumber)
		return nil
	}

	ghHandler := gh.NewHandler([]byte(webhookSecret), createTask)

	mux := http.NewServeMux()
	mux.Handle("/webhook/github", ghHandler)

	// Paperless webhook config — enabled when PAPERLESS_API_URL is set.
	// In the standard K8s deployment this is always set; the guard exists for
	// local development or alternative deployments without Paperless.
	paperlessAPIURL := os.Getenv("PAPERLESS_API_URL")
	if paperlessAPIURL != "" {
		paperlessToken := os.Getenv("PAPERLESS_API_TOKEN")
		if paperlessToken == "" {
			log.Fatal("PAPERLESS_API_TOKEN required when PAPERLESS_API_URL is set")
		}
		paperlessSecret := os.Getenv("PAPERLESS_WEBHOOK_SECRET")
		if paperlessSecret == "" {
			log.Fatal("PAPERLESS_WEBHOOK_SECRET required when PAPERLESS_API_URL is set")
		}
		paperlessDomain := os.Getenv("PAPERLESS_DOMAIN")
		if paperlessDomain == "" {
			log.Fatal("PAPERLESS_DOMAIN required when PAPERLESS_API_URL is set")
		}
		localaiURL := os.Getenv("LOCALAI_URL")
		if localaiURL == "" {
			log.Fatal("LOCALAI_URL required when PAPERLESS_API_URL is set")
		}
		localaiModel := os.Getenv("LOCALAI_MODEL")
		if localaiModel == "" {
			localaiModel = "phi-3-mini"
		}
		vaultPath := os.Getenv("VAULT_PATH")
		if vaultPath == "" {
			log.Fatal("VAULT_PATH required when PAPERLESS_API_URL is set")
		}
		searchURL := os.Getenv("AIOS_SEARCH_URL")
		if searchURL == "" {
			log.Fatal("AIOS_SEARCH_URL required when PAPERLESS_API_URL is set")
		}

		paperlessHandler := paperless.NewHandler(
			paperlessSecret,
			paperlessAPIURL,
			paperlessToken,
			paperlessDomain,
			localaiURL,
			localaiModel,
			vaultPath,
			searchURL,
		)
		mux.Handle("/webhook/paperless", paperlessHandler)
		log.Printf("Paperless webhook enabled at /webhook/paperless")
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGTERM/SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		log.Printf("webhook server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down webhook server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}

	log.Println("webhook server stopped")
}
