package auth

import (
	"testing"
	"time"
)

func TestStateFromTokenAge(t *testing.T) {
	tests := []struct {
		name    string
		ageDays float64
		want    State
	}{
		{"fresh", 1, StateHealthy},
		{"approaching", 24, StateWarning}, // 7d before 30d expiry
		{"expired", 31, StateExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry := time.Now().Add(time.Duration((30 - tt.ageDays) * 24 * float64(time.Hour)))
			got := StateFromExpiry(expiry, time.Now())
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransitionAwaiting(t *testing.T) {
	m := NewMachine()
	m.Set(StateHealthy)
	if err := m.Transition(StateAwaiting); err != nil {
		t.Fatal(err)
	}
	if m.State() != StateAwaiting {
		t.Fatalf("not transitioned")
	}
}

func TestTransitionRejectsInvalid(t *testing.T) {
	m := NewMachine()
	m.Set(StateAwaiting)
	if err := m.Transition(StateWarning); err == nil {
		t.Fatal("should reject Awaiting->Warning")
	}
}
