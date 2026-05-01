package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Diixtra/aios/auth-broker/internal/store"
)

const bundleSizeLimit = 1 << 20 // 1 MiB — pi auth.json is small (KB range)

// BundleHandler accepts an auth.json upload and triggers re-validation.
//
// Schema is intentionally not validated — pi is the source of truth for
// auth.json structure. We require only that the body is valid JSON so we
// fail fast on `curl -F bundle=@/wrong/file`.
type BundleHandler struct {
	store           *store.Store
	triggerValidate func()
}

func NewBundleHandler(s *store.Store, triggerValidate func()) *BundleHandler {
	return &BundleHandler{store: s, triggerValidate: triggerValidate}
}

func (h *BundleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, bundleSizeLimit+1)
	if err := r.ParseMultipartForm(bundleSizeLimit); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "bundle too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid multipart body", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "missing bundle field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if !json.Valid(raw) {
		http.Error(w, "bundle must be JSON", http.StatusBadRequest)
		return
	}
	if err := h.store.Write(store.Bundle{Raw: raw}); err != nil {
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	if h.triggerValidate != nil {
		h.triggerValidate()
	}
	w.WriteHeader(http.StatusAccepted)
}
