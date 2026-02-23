package monitor

import (
	"testing"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateUnknown, "unknown"},
		{StateUp, "up"},
		{StateDown, "down"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	cfg.defaults()

	if cfg.Target != "1.1.1.1" {
		t.Errorf("default target = %q, want 1.1.1.1", cfg.Target)
	}
	if cfg.Interval != 3*time.Second {
		t.Errorf("default interval = %v, want 3s", cfg.Interval)
	}
	if cfg.Threshold != 3 {
		t.Errorf("default threshold = %d, want 3", cfg.Threshold)
	}
}

func TestConfigDefaultsPreserve(t *testing.T) {
	cfg := Config{
		Target:    "8.8.8.8",
		Interval:  5 * time.Second,
		Threshold: 5,
	}
	cfg.defaults()

	if cfg.Target != "8.8.8.8" {
		t.Errorf("target = %q, want 8.8.8.8", cfg.Target)
	}
	if cfg.Interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", cfg.Interval)
	}
	if cfg.Threshold != 5 {
		t.Errorf("threshold = %d, want 5", cfg.Threshold)
	}
}

func TestHandleRecvTransitionsToUp(t *testing.T) {
	var events []Event
	m := New(Config{
		OnStateChange: func(e Event) {
			events = append(events, e)
		},
	})

	// Force down state
	m.mu.Lock()
	m.state = StateDown
	m.downSince = time.Now().Add(-10 * time.Second)
	m.mu.Unlock()

	m.handleRecv(&probing.Packet{Seq: 1, Rtt: 10 * time.Millisecond})

	if m.State() != StateUp {
		t.Errorf("state = %v, want up", m.State())
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].State != StateUp {
		t.Errorf("event state = %v, want up", events[0].State)
	}
	if events[0].Duration < 10*time.Second {
		t.Errorf("outage duration = %v, want >= 10s", events[0].Duration)
	}
}

func TestHandleFailureTransitionsToDown(t *testing.T) {
	var events []Event
	m := New(Config{
		Threshold: 3,
		OnStateChange: func(e Event) {
			events = append(events, e)
		},
	})

	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.handleFailure()
	m.handleFailure()
	if m.State() != StateUp {
		t.Errorf("state after 2 failures = %v, want up", m.State())
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after 2 failures, got %d", len(events))
	}

	m.handleFailure()
	if m.State() != StateDown {
		t.Errorf("state after 3 failures = %v, want down", m.State())
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].State != StateDown {
		t.Errorf("event state = %v, want down", events[0].State)
	}
}

func TestHandleRecvResetsFailureCount(t *testing.T) {
	m := New(Config{Threshold: 3})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.handleFailure()
	m.handleFailure()
	m.handleRecv(&probing.Packet{Seq: 1, Rtt: 5 * time.Millisecond})

	m.handleFailure()
	m.handleFailure()
	if m.State() != StateUp {
		t.Errorf("state = %v, want up (failures should have been reset)", m.State())
	}
}

func TestOnPingCallback(t *testing.T) {
	var called bool
	m := New(Config{
		OnPing: func(seq int, rtt time.Duration) {
			called = true
		},
	})

	m.handleRecv(&probing.Packet{Seq: 1, Rtt: 5 * time.Millisecond})
	if !called {
		t.Error("OnPing was not called")
	}
}

func TestNoCallbackOnFirstUp(t *testing.T) {
	var events []Event
	m := New(Config{
		OnStateChange: func(e Event) {
			events = append(events, e)
		},
	})

	// State is Unknown, first recv should set to Up but not fire callback
	// (only fires on transitions from Down->Up)
	m.handleRecv(&probing.Packet{Seq: 1, Rtt: 5 * time.Millisecond})
	if len(events) != 0 {
		t.Errorf("expected no events on first up, got %d", len(events))
	}
}
