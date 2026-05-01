package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSlack struct {
	calls []string
	err   error
}

func (f *fakeSlack) DM(_ context.Context, userID, text string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, userID+":"+text)
	return nil
}

func TestNotifier_BootstrapRecipe(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.BootstrapRecipe(context.Background(), "tick-validation-failed"); err != nil {
		t.Fatal(err)
	}
	if len(c.calls) != 1 {
		t.Fatalf("got %v", c.calls)
	}
	if !strings.Contains(c.calls[0], "pi /login") || !strings.Contains(c.calls[0], "tick-validation-failed") {
		t.Fatalf("DM missing recipe or reason: %s", c.calls[0])
	}
}

func TestNotifier_Warning(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.Warning(context.Background(), 25); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.calls[0], "25") {
		t.Fatalf("Warning DM missing age: %s", c.calls[0])
	}
}

func TestNotifier_Recovered(t *testing.T) {
	c := &fakeSlack{}
	n := NewNotifier(c, "U123", "https://auth-broker.aios.local")
	if err := n.Recovered(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.calls[0], "reauthenticated") {
		t.Fatalf("Recovered DM unexpected: %s", c.calls[0])
	}
}

func TestNotifier_PropagatesError(t *testing.T) {
	c := &fakeSlack{err: errors.New("boom")}
	n := NewNotifier(c, "U123", "https://x")
	if err := n.Recovered(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
