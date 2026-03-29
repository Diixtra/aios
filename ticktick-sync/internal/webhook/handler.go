package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// SyncEngine defines the sync operations the webhook handler can trigger.
type SyncEngine interface {
	HandleIssueClosed(ctx context.Context, repo string, number int) error
	HandleIssueLabeled(ctx context.Context, repo string, number int, title, body string) error
}

// Handler processes GitHub webhook events for the sync service.
type Handler struct {
	secret []byte
	engine SyncEngine
}

// NewHandler creates a new webhook handler.
func NewHandler(secret []byte, engine SyncEngine) *Handler {
	return &Handler{secret: secret, engine: engine}
}

type issueEvent struct {
	Action string `json:"action"`
	Label  struct {
		Name string `json:"name"`
	} `json:"label"`
	Issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	signature := r.Header.Get("X-Hub-Signature-256")
	if !validSignature(h.secret, body, signature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "issues" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ignored event")
		return
	}

	var event issueEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	switch event.Action {
	case "closed":
		slog.Info("webhook: issue closed", "repo", event.Repository.FullName, "issue", event.Issue.Number)
		if err := h.engine.HandleIssueClosed(ctx, event.Repository.FullName, event.Issue.Number); err != nil {
			slog.Error("webhook: handle issue closed failed", "error", err)
			http.Error(w, "sync failed", http.StatusInternalServerError)
			return
		}
	case "labeled":
		if event.Label.Name != "agent" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ignored label")
			return
		}
		slog.Info("webhook: issue labeled agent", "repo", event.Repository.FullName, "issue", event.Issue.Number)
		if err := h.engine.HandleIssueLabeled(ctx, event.Repository.FullName, event.Issue.Number, event.Issue.Title, event.Issue.Body); err != nil {
			slog.Error("webhook: handle issue labeled failed", "error", err)
			http.Error(w, "sync failed", http.StatusInternalServerError)
			return
		}
	default:
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ignored action")
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func validSignature(secret, payload []byte, signature string) bool {
	if len(signature) < 7 || signature[:7] != "sha256=" {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hmac.Equal(sig, mac.Sum(nil))
}
