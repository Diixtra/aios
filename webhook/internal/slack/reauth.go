// Package slack contains webhook handlers that bridge Slack into AIOS.
package slack

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ReauthHandler proxies the /aios-reauth slash command to the auth-broker's
// /v1/admin/revalidate endpoint and returns the resulting state to Slack.
// Triggering revalidate also causes the broker to re-emit the bootstrap recipe
// DM (per orchestrator contract in auth-broker Task B8) so the user gets the
// laptop instructions in their DM.
type ReauthHandler struct {
	authBrokerURL string
	adminToken    string
	signingSecret []byte
	client        *http.Client
}

// NewReauthHandler constructs the handler. signingSecret is the Slack app's
// signing secret used to verify incoming requests; pass an empty string to
// disable verification (only acceptable in local dev).
func NewReauthHandler(authBrokerURL, adminToken, signingSecret string) *ReauthHandler {
	return &ReauthHandler{
		authBrokerURL: authBrokerURL,
		adminToken:    adminToken,
		signingSecret: []byte(signingSecret),
		client:        &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *ReauthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if err := h.verifySlackSignature(r, body); err != nil {
		log.Printf("slack reauth: signature rejected: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		h.authBrokerURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := h.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	var body2 struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body2)
	text := "AIOS auth state: " + body2.State
	if body2.Message != "" {
		text += " — " + body2.Message
	}
	if body2.State != "Healthy" {
		text += "\n(Bootstrap recipe DM'd to you; re-run `pi /login` on laptop and `just bootstrap-auth`.)"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"response_type": "ephemeral",
		"text":          text,
	})
}

// verifySlackSignature validates the request per Slack's signing-secret spec.
// https://api.slack.com/authentication/verifying-requests-from-slack
func (h *ReauthHandler) verifySlackSignature(r *http.Request, body []byte) error {
	if len(h.signingSecret) == 0 {
		return nil // explicitly disabled (dev/test only)
	}
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	got := r.Header.Get("X-Slack-Signature")
	if ts == "" || got == "" {
		return fmt.Errorf("missing slack signature headers")
	}
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("bad timestamp")
	}
	if abs(time.Now().Unix()-tsInt) > 60*5 {
		return fmt.Errorf("timestamp too old")
	}
	mac := hmac.New(sha256.New, h.signingSecret)
	fmt.Fprintf(mac, "v0:%s:%s", ts, body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(got)) {
		return fmt.Errorf("signature mismatch")
	}
	// Restore body for downstream consumption (we already read it).
	r.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
