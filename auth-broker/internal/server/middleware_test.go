package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeReviewer struct {
	ok      bool
	subject string
}

func (f *fakeReviewer) Authenticate(_ context.Context, _ string) (string, error) {
	if f.ok {
		return f.subject, nil
	}
	return "", errors.New("not authenticated")
}

var passthrough = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestRequireSAToken_RejectsMissing(t *testing.T) {
	h := requireSAToken(&fakeReviewer{}, passthrough)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRequireSAToken_RejectsInvalid(t *testing.T) {
	h := requireSAToken(&fakeReviewer{ok: false}, passthrough)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRequireSAToken_AllowsValid(t *testing.T) {
	h := requireSAToken(&fakeReviewer{ok: true, subject: "system:serviceaccount:aios:agent-task"}, passthrough)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRequireAdminToken_RejectsMismatch(t *testing.T) {
	h := requireAdminToken("expected", passthrough)
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRequireAdminToken_AllowsMatch(t *testing.T) {
	h := requireAdminToken("expected", passthrough)
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Bearer expected")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRequireAdminToken_RejectsEmptyExpected(t *testing.T) {
	// Empty admin token should refuse all requests rather than auto-allowing.
	h := requireAdminToken("", passthrough)
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}
