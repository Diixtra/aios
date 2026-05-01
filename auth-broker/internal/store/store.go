package store

import (
	"errors"
	"os"
	"path/filepath"
)

type Bundle struct {
	// Raw is the verbatim contents of pi's auth.json. We treat it as opaque to
	// the broker; only pi parses the inner structure. Phase -1 task A2 confirmed
	// the file shape; if its semantics change we update the spike doc, not this.
	Raw []byte
}

type Store struct {
	path string
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Read() (Bundle, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{Raw: b}, nil
}

// Write atomically replaces the bundle on disk.
func (s *Store) Write(b Bundle) error {
	if len(b.Raw) == 0 {
		return errors.New("store: refusing to write empty bundle")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(b.Raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
