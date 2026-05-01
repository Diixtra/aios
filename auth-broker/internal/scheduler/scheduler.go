package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Job func(context.Context) error

type Scheduler struct {
	interval time.Duration
	job      Job
}

func New(interval time.Duration, job Job) *Scheduler {
	return &Scheduler{interval: interval, job: job}
}

// Run blocks until ctx is cancelled, invoking the job on each tick.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.job(ctx); err != nil {
				slog.Warn("scheduled job failed", "err", err)
			}
		}
	}
}
