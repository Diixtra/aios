package auth

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateUnknown  State = "Unknown"
	StateHealthy  State = "Healthy"
	StateWarning  State = "Warning"
	StateExpired  State = "Expired"
	StateAwaiting State = "Awaiting"
)

// validTransitions[from] = allowed targets
var validTransitions = map[State]map[State]struct{}{
	StateUnknown:  {StateHealthy: {}, StateExpired: {}, StateAwaiting: {}},
	StateHealthy:  {StateWarning: {}, StateExpired: {}, StateAwaiting: {}},
	StateWarning:  {StateHealthy: {}, StateExpired: {}, StateAwaiting: {}},
	StateExpired:  {StateAwaiting: {}, StateHealthy: {}},
	StateAwaiting: {StateHealthy: {}, StateExpired: {}},
}

type Machine struct {
	mu    sync.RWMutex
	state State
}

func NewMachine() *Machine {
	return &Machine{state: StateUnknown}
}

func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Set forces the state without validation. Use for initial load only.
func (m *Machine) Set(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *Machine) Transition(to State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed, ok := validTransitions[m.state]
	if !ok {
		return errors.New("auth: state has no allowed transitions")
	}
	if _, ok := allowed[to]; !ok {
		return errors.New("auth: invalid transition " + string(m.state) + "->" + string(to))
	}
	m.state = to
	return nil
}

// StateFromExpiry classifies expiry-relative health.
//
// >7 days remaining -> Healthy; 0-7 days -> Warning; <=0 -> Expired.
func StateFromExpiry(expiresAt, now time.Time) State {
	delta := expiresAt.Sub(now)
	switch {
	case delta <= 0:
		return StateExpired
	case delta <= 7*24*time.Hour:
		return StateWarning
	default:
		return StateHealthy
	}
}
