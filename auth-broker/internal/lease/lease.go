package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Lease struct {
	ID        string
	Holder    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type Manager struct {
	cap int
	ttl time.Duration

	mu     sync.Mutex
	active map[string]*Lease
	wakeup chan struct{}
}

func New(capacity int, ttl time.Duration) *Manager {
	return &Manager{
		cap:    capacity,
		ttl:    ttl,
		active: make(map[string]*Lease),
		wakeup: make(chan struct{}, 1),
	}
}

func (m *Manager) Acquire(ctx context.Context, holder string) (*Lease, error) {
	for {
		m.mu.Lock()
		m.reapLocked(time.Now())
		if len(m.active) < m.cap {
			id := newID()
			now := time.Now()
			l := &Lease{ID: id, Holder: holder, IssuedAt: now, ExpiresAt: now.Add(m.ttl)}
			m.active[id] = l
			m.mu.Unlock()
			return l, nil
		}
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.wakeup:
			// Try again.
		case <-time.After(50 * time.Millisecond):
			// Periodic re-check (catches lease expiry).
		}
	}
}

func (m *Manager) Release(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[id]; !ok {
		return errors.New("lease: unknown id")
	}
	delete(m.active, id)
	select {
	case m.wakeup <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reapLocked(time.Now())
	return len(m.active)
}

func (m *Manager) reapLocked(now time.Time) {
	for id, l := range m.active {
		if now.After(l.ExpiresAt) {
			delete(m.active, id)
		}
	}
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
