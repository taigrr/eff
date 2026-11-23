package monitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunRejectsFinishedContextBeforeResolvingTarget(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()

	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "canceled", ctx: canceled, want: context.Canceled},
		{name: "expired", ctx: expired, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Invalid syntax ensures DNS/socket setup cannot mask cancellation.
			monitor := New(Config{Target: "invalid target\x00"})
			if err := monitor.Run(test.ctx); !errors.Is(err, test.want) {
				t.Fatalf("Run() error = %v, want %v", err, test.want)
			}
			if monitor.State() != StateUnknown || monitor.Stats() != (Stats{}) {
				t.Fatal("finished context changed monitor state or statistics")
			}
		})
	}
}
