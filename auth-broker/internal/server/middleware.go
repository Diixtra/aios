package server

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type tokenReviewer interface {
	Authenticate(ctx context.Context, token string) (subject string, err error)
}

type ctxSubject struct{}

func requireSAToken(rv tokenReviewer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		sub, err := rv.Authenticate(r.Context(), tok)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxSubject{}, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdminToken(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		// constant-time compare; refuse on empty expected to avoid auto-allow.
		if expected == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// KubeReviewer authenticates bearer tokens via the Kubernetes TokenReview API.
type KubeReviewer struct {
	Client kubernetes.Interface
}

func (k *KubeReviewer) Authenticate(ctx context.Context, token string) (string, error) {
	tr := &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: token}}
	res, err := k.Client.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	if !res.Status.Authenticated {
		return "", &reviewError{msg: "not authenticated"}
	}
	return res.Status.User.Username, nil
}

type reviewError struct{ msg string }

func (e *reviewError) Error() string { return e.msg }
