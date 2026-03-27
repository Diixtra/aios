package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// signPayload generates a valid X-Hub-Signature-256 header value for the given payload and secret.
func signPayload(payload, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandler_AgentLabeledEvent(t *testing.T) {
	secret := []byte("test-secret")
	var captured *TaskRequest

	handler := NewHandler(secret, func(req TaskRequest) error {
		captured = &req
		return nil
	})

	payload := map[string]interface{}{
		"action": "labeled",
		"label": map[string]string{
			"name": "agent",
		},
		"issue": map[string]interface{}{
			"number": 42,
			"title":  "Implement feature X",
			"body":   "Please implement feature X with tests.",
		},
		"repository": map[string]string{
			"full_name": "Diixtra/aios",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", signPayload(body, secret))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	if captured == nil {
		t.Fatal("expected task to be created, but callback was not called")
	}

	if captured.Repo != "Diixtra/aios" {
		t.Errorf("expected repo 'Diixtra/aios', got %q", captured.Repo)
	}
	if captured.IssueNumber != 42 {
		t.Errorf("expected issue number 42, got %d", captured.IssueNumber)
	}
	if captured.Title != "Implement feature X" {
		t.Errorf("expected title 'Implement feature X', got %q", captured.Title)
	}
	if captured.Body != "Please implement feature X with tests." {
		t.Errorf("expected body 'Please implement feature X with tests.', got %q", captured.Body)
	}
}

func TestHandler_NonAgentLabel(t *testing.T) {
	secret := []byte("test-secret")
	taskCreated := false

	handler := NewHandler(secret, func(req TaskRequest) error {
		taskCreated = true
		return nil
	})

	payload := map[string]interface{}{
		"action": "labeled",
		"label": map[string]string{
			"name": "bug",
		},
		"issue": map[string]interface{}{
			"number": 10,
			"title":  "Bug report",
			"body":   "Something is broken.",
		},
		"repository": map[string]string{
			"full_name": "Diixtra/aios",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", signPayload(body, secret))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if taskCreated {
		t.Error("expected no task to be created for non-agent label")
	}
}

func TestHandler_InvalidSignature(t *testing.T) {
	secret := []byte("test-secret")
	taskCreated := false

	handler := NewHandler(secret, func(req TaskRequest) error {
		taskCreated = true
		return nil
	})

	payload := []byte(`{"action":"labeled","label":{"name":"agent"}}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignaturevalue0000000000000000000000000000000000000000000")
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}

	if taskCreated {
		t.Error("expected no task to be created with invalid signature")
	}
}
