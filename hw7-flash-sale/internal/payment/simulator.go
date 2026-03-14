package payment

import (
	"context"
	"time"
)

type Simulator struct {
	slots chan struct{}
	delay time.Duration
}

func NewSimulator(capacity int, delay time.Duration) *Simulator {
	return &Simulator{
		slots: make(chan struct{}, capacity),
		delay: delay,
	}
}

func (s *Simulator) Verify(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	defer func() {
		<-s.slots
	}()

	timer := time.NewTimer(s.delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
