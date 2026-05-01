package auth

import (
	"context"
	"errors"
)

const defaultWarnAgeDays = 23 // OAuth refresh-token chains commonly survive ~30d

type validator interface {
	Validate(context.Context) error
}

type Notifier interface {
	BootstrapRecipe(ctx context.Context, reason string) error
	Recovered(ctx context.Context) error
	Warning(ctx context.Context, ageDays int) error
}

type Orchestrator struct {
	sm        *Machine
	validator validator
	notifier  Notifier

	// bundleAge returns (ageDays, present). Wired by main to a closure over
	// the store's mtime; injectable for tests.
	bundleAge   func() (int, bool)
	warnAgeDays int
}

func NewOrchestrator(sm *Machine, v validator, n Notifier) *Orchestrator {
	return &Orchestrator{sm: sm, validator: v, notifier: n, warnAgeDays: defaultWarnAgeDays}
}

// SetBundleAge wires the bundle-age probe used by Tick. Must be called before
// the first Tick; the in-cluster wiring in main.go does this at startup.
func (o *Orchestrator) SetBundleAge(fn func() (ageDays int, present bool)) {
	o.bundleAge = fn
}

// OnBundleUploaded validates the freshly-stored bundle and transitions state.
func (o *Orchestrator) OnBundleUploaded(ctx context.Context) error {
	prev := o.sm.State()
	if err := o.validator.Validate(ctx); err != nil {
		_ = o.sm.Transition(StateExpired)
		_ = o.notifier.BootstrapRecipe(ctx, "upload-validation-failed")
		return err
	}
	_ = o.sm.Transition(StateHealthy)
	if prev != StateHealthy {
		_ = o.notifier.Recovered(ctx)
	}
	return nil
}

// Tick runs the periodic check. Should be called by the B5 scheduler.
func (o *Orchestrator) Tick(ctx context.Context) error {
	if o.bundleAge == nil {
		return errors.New("orchestrator: bundleAge not wired")
	}
	age, present := o.bundleAge()
	if !present {
		// No bundle uploaded yet. Nothing to do — bootstrap recipe was sent
		// at startup; do not re-DM on every tick.
		return nil
	}
	prev := o.sm.State()
	if err := o.validator.Validate(ctx); err != nil {
		if prev != StateExpired {
			_ = o.sm.Transition(StateExpired)
			_ = o.notifier.BootstrapRecipe(ctx, "tick-validation-failed")
		}
		return err
	}
	if age >= o.warnAgeDays {
		if prev != StateWarning {
			_ = o.sm.Transition(StateWarning)
			_ = o.notifier.Warning(ctx, age)
		}
		return nil
	}
	// Healthy.
	if prev != StateHealthy {
		_ = o.sm.Transition(StateHealthy)
		if prev == StateExpired || prev == StateWarning {
			_ = o.notifier.Recovered(ctx)
		}
	}
	return nil
}
