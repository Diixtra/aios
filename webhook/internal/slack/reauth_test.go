package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sign(secret, body string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%d:%s", ts, body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestReauth_CallsAuthBrokerAndReturnsState(t *testing.T) {
	captured := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- string(body)
		_, _ = w.Write([]byte(`{"state":"Healthy","message":"validated ok"}`))
	}))
	defer srv.Close()

	h := NewReauthHandler(srv.URL+"/v1/admin/revalidate", "admin-token", "")
	rec := httptest.NewRecorder()
	form := url.Values{"text": []string{""}}
	req := httptest.NewRequest(http.MethodPost, "/slack/reauth",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
	select {
	case <-captured:
	case <-time.After(time.Second):
		t.Fatal("auth-broker never called")
	}
	if !strings.Contains(rec.Body.String(), "Healthy") {
		t.Fatalf("expected state in response: %s", rec.Body)
	}
}

func TestReauth_RejectsBadSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"Healthy"}`))
	}))
	defer srv.Close()

	h := NewReauthHandler(srv.URL, "admin-token", "secret")
	rec := httptest.NewRecorder()
	body := "text="
	req := httptest.NewRequest(http.MethodPost, "/slack/reauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Slack-Signature", "v0=deadbeef")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
}

func TestReauth_AcceptsValidSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"Healthy"}`))
	}))
	defer srv.Close()

	const secret = "secret"
	body := "text="
	ts := time.Now().Unix()
	sig := sign(secret, body, ts)

	h := NewReauthHandler(srv.URL, "admin-token", secret)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/slack/reauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Slack-Signature", sig)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestReauth_RejectsStaleTimestamp(t *testing.T) {
	h := NewReauthHandler("", "admin-token", "secret")
	rec := httptest.NewRecorder()
	body := "text="
	stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/slack/reauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", stale)
	req.Header.Set("X-Slack-Signature", "v0=any")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}
