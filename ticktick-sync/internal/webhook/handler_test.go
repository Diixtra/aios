package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockEngine struct {
	closedRepo   string
	closedNumber int
	labeledRepo  string
	labeledNum   int
	labeledTitle string
	labeledBody  string
}

func (m *mockEngine) HandleIssueClosed(ctx context.Context, repo string, number int) error {
	m.closedRepo = repo
	m.closedNumber = number
	return nil
}

func (m *mockEngine) HandleIssueLabeled(ctx context.Context, repo string, number int, title, body string) error {
	m.labeledRepo = repo
	m.labeledNum = number
	m.labeledTitle = title
	m.labeledBody = body
	return nil
}

func sign(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandlerIssueClosed(t *testing.T) {
	engine := &mockEngine{}
	h := NewHandler([]byte("secret"), engine)

	event := map[string]interface{}{
		"action":     "closed",
		"issue":      map[string]interface{}{"number": 42, "title": "Test", "body": "desc"},
		"repository": map[string]interface{}{"full_name": "Diixtra/aios"},
	}
	body, _ := json.Marshal(event)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("secret"), body))
	req.Header.Set("X-GitHub-Event", "issues")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if engine.closedRepo != "Diixtra/aios" || engine.closedNumber != 42 {
		t.Errorf("closed %s#%d, want Diixtra/aios#42", engine.closedRepo, engine.closedNumber)
	}
}

func TestHandlerIssueLabeled(t *testing.T) {
	engine := &mockEngine{}
	h := NewHandler([]byte("secret"), engine)

	event := map[string]interface{}{
		"action":     "labeled",
		"label":      map[string]interface{}{"name": "agent"},
		"issue":      map[string]interface{}{"number": 10, "title": "New feat", "body": "build it"},
		"repository": map[string]interface{}{"full_name": "Diixtra/aios"},
	}
	body, _ := json.Marshal(event)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("secret"), body))
	req.Header.Set("X-GitHub-Event", "issues")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if engine.labeledTitle != "New feat" {
		t.Errorf("title = %q, want %q", engine.labeledTitle, "New feat")
	}
}

func TestHandlerInvalidSignature(t *testing.T) {
	engine := &mockEngine{}
	h := NewHandler([]byte("secret"), engine)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader("{}"))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "issues")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandlerIgnoresNonIssueEvents(t *testing.T) {
	engine := &mockEngine{}
	h := NewHandler([]byte("secret"), engine)

	body := []byte("{}")
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("secret"), body))
	req.Header.Set("X-GitHub-Event", "push")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
