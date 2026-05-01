package auth

import (
	"context"
	"errors"
	"testing"
)

type fakeNotifier struct {
	calls []string
}

func (f *fakeNotifier) BootstrapRecipe(_ context.Context, reason string) error {
	f.calls = append(f.calls, "bootstrap:"+reason)
	return nil
}
func (f *fakeNotifier) Recovered(_ context.Context) error {
	f.calls = append(f.calls, "recovered")
	return nil
}
func (f *fakeNotifier) Warning(_ context.Context, _ int) error {
	f.calls = append(f.calls, "warning")
	return nil
}

type fakeValidator struct{ err error }

func (f *fakeValidator) Validate(_ context.Context) error { return f.err }

func TestOrchestrator_UploadOK_TransitionsToHealthyAndDMsRecovery(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateExpired)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)

	if err := o.OnBundleUploaded(context.Background()); err != nil {
		t.Fatalf("OnBundleUploaded: %v", err)
	}
	if sm.State() != StateHealthy {
		t.Fatalf("state=%s want Healthy", sm.State())
	}
	if len(n.calls) != 1 || n.calls[0] != "recovered" {
		t.Fatalf("notify=%v want [recovered]", n.calls)
	}
}

func TestOrchestrator_UploadFails_TransitionsToExpiredAndDMsBootstrap(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateHealthy)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{err: errors.New("auth error")}, n)

	if err := o.OnBundleUploaded(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if sm.State() != StateExpired {
		t.Fatalf("state=%s want Expired", sm.State())
	}
	if len(n.calls) != 1 || n.calls[0] != "bootstrap:upload-validation-failed" {
		t.Fatalf("notify=%v", n.calls)
	}
}

func TestOrchestrator_Tick_NoBundle_NoTransition(t *testing.T) {
	sm := NewMachine()
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)
	o.bundleAge = func() (int, bool) { return 0, false } // no bundle
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("expected no DMs; got %v", n.calls)
	}
}

func TestOrchestrator_Tick_AgedBundle_TransitionsToWarningOnce(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateHealthy)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)
	o.bundleAge = func() (int, bool) { return 25, true } // past warn (default 23d)
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sm.State() != StateWarning {
		t.Fatalf("state=%s want Warning", sm.State())
	}
	// Two ticks, one DM — transition fires only once.
	if len(n.calls) != 1 || n.calls[0] != "warning" {
		t.Fatalf("notify=%v want [warning]", n.calls)
	}
}

func TestOrchestrator_Tick_ValidationFails_TransitionsToExpiredOnce(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateHealthy)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{err: errors.New("auth")}, n)
	o.bundleAge = func() (int, bool) { return 1, true }
	_ = o.Tick(context.Background())
	_ = o.Tick(context.Background())
	if sm.State() != StateExpired {
		t.Fatalf("state=%s want Expired", sm.State())
	}
	if len(n.calls) != 1 {
		t.Fatalf("expected single bootstrap DM; got %v", n.calls)
	}
}

func TestOrchestrator_Tick_RecoveryFromExpired_DMsRecovered(t *testing.T) {
	sm := NewMachine()
	sm.Set(StateExpired)
	n := &fakeNotifier{}
	o := NewOrchestrator(sm, &fakeValidator{}, n)
	o.bundleAge = func() (int, bool) { return 1, true }
	if err := o.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sm.State() != StateHealthy {
		t.Fatalf("state=%s want Healthy", sm.State())
	}
	if len(n.calls) != 1 || n.calls[0] != "recovered" {
		t.Fatalf("notify=%v", n.calls)
	}
}
