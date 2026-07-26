package monitor

import (
	"errors"
	"net"
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

func TestConfigNormalize(t *testing.T) {
	cfg := Config{}
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

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

func TestConfigNormalizePreservesExplicitValues(t *testing.T) {
	cfg := Config{
		Target:    "8.8.8.8",
		Interval:  5 * time.Second,
		Threshold: 5,
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

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

func TestConfigValidateRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "empty target",
			cfg:  Config{Target: "", Interval: time.Second, Threshold: 1},
		},
		{
			name: "negative interval",
			cfg:  Config{Target: "1.1.1.1", Interval: -time.Second, Threshold: 1},
		},
		{
			name: "zero threshold",
			cfg:  Config{Target: "1.1.1.1", Interval: time.Second, Threshold: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want non-nil")
			}
		})
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

func TestNewAppliesDefaults(t *testing.T) {
	m := New(Config{})
	if m.cfg.Target != "1.1.1.1" {
		t.Errorf("target = %q, want 1.1.1.1", m.cfg.Target)
	}
	if m.state != StateUnknown {
		t.Errorf("initial state = %v, want unknown", m.state)
	}
}

func TestStatsReturnsSnapshot(t *testing.T) {
	m := New(Config{})
	m.mu.Lock()
	m.stats = Stats{
		PacketsSent: 100,
		PacketsRecv: 95,
		PacketLoss:  5.0,
		MinRTT:      1 * time.Millisecond,
		AvgRTT:      5 * time.Millisecond,
		MaxRTT:      20 * time.Millisecond,
	}
	m.mu.Unlock()

	s := m.Stats()
	if s.PacketsSent != 100 {
		t.Errorf("PacketsSent = %d, want 100", s.PacketsSent)
	}
	if s.PacketsRecv != 95 {
		t.Errorf("PacketsRecv = %d, want 95", s.PacketsRecv)
	}
	if s.PacketLoss != 5.0 {
		t.Errorf("PacketLoss = %f, want 5.0", s.PacketLoss)
	}
}

func TestDownStayDownOnMoreFailures(t *testing.T) {
	var eventCount int
	m := New(Config{
		Threshold: 2,
		OnStateChange: func(e Event) {
			eventCount++
		},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	// Trigger transition to down
	m.handleFailure()
	m.handleFailure()
	if eventCount != 1 {
		t.Fatalf("expected 1 event after threshold, got %d", eventCount)
	}

	// Additional failures should NOT fire more events
	m.handleFailure()
	m.handleFailure()
	m.handleFailure()
	if eventCount != 1 {
		t.Errorf("expected still 1 event after extra failures, got %d", eventCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := New(Config{
		Threshold:     3,
		OnStateChange: func(e Event) {},
		OnPing:        func(seq int, rtt time.Duration) {},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	done := make(chan struct{})

	// Concurrent failures
	go func() {
		for range 100 {
			m.handleFailure()
		}
		done <- struct{}{}
	}()

	// Concurrent receives
	go func() {
		for range 100 {
			m.handleRecv(&probing.Packet{Seq: 1, Rtt: time.Millisecond})
		}
		done <- struct{}{}
	}()

	// Concurrent state reads
	go func() {
		for range 100 {
			_ = m.State()
			_ = m.Stats()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done
}

func TestUpEventIncludesDownSince(t *testing.T) {
	var event Event
	m := New(Config{
		OnStateChange: func(e Event) {
			event = e
		},
	})

	downTime := time.Now().Add(-30 * time.Second)
	m.mu.Lock()
	m.state = StateDown
	m.downSince = downTime
	m.mu.Unlock()

	m.handleRecv(&probing.Packet{Seq: 1, Rtt: time.Millisecond})

	if event.DownSince != downTime {
		t.Errorf("DownSince = %v, want %v", event.DownSince, downTime)
	}
	if event.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", event.Target)
	}
}

func TestThresholdOneImmediateDown(t *testing.T) {
	var events []Event
	m := New(Config{
		Threshold: 1,
		OnStateChange: func(e Event) {
			events = append(events, e)
		},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.handleFailure()
	if m.State() != StateDown {
		t.Errorf("state = %v, want down with threshold=1", m.State())
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestConfigNormalizeAcceptsDefaults(t *testing.T) {
	cfg := Config{}
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func TestRecvTimeoutErrorCountsFailureWithoutCallback(t *testing.T) {
	var (
		events     []Event
		errorCalls int
	)

	m := New(Config{
		Threshold: 1,
		OnStateChange: func(e Event) {
			events = append(events, e)
		},
		OnError: func(err error) {
			errorCalls++
		},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.pingerRecvError(&net.OpError{Err: timeoutErr{}})

	if m.State() != StateDown {
		t.Fatalf("state = %v, want down", m.State())
	}
	if errorCalls != 0 {
		t.Fatalf("OnError calls = %d, want 0", errorCalls)
	}
	if len(events) != 1 || events[0].State != StateDown {
		t.Fatalf("events = %+v, want single down event", events)
	}
}

func TestRecvNonTimeoutErrorInvokesCallback(t *testing.T) {
	var got error
	m := New(Config{
		Threshold: 2,
		OnError: func(err error) {
			got = err
		},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.pingerRecvError(errors.New("boom"))

	if got == nil {
		t.Fatal("OnError was not called")
	}
	if got.Error() != "recv: boom" {
		t.Fatalf("OnError = %q, want %q", got.Error(), "recv: boom")
	}
	if m.State() != StateUp {
		t.Fatalf("state = %v, want up before threshold", m.State())
	}
}

func TestSendErrorInvokesCallbackAndCountsFailure(t *testing.T) {
	var got error
	m := New(Config{
		Threshold: 1,
		OnError: func(err error) {
			got = err
		},
	})
	m.mu.Lock()
	m.state = StateUp
	m.mu.Unlock()

	m.pingerSendError(&probing.Packet{}, errors.New("send failed"))

	if got == nil {
		t.Fatal("OnError was not called")
	}
	if got.Error() != "send: send failed" {
		t.Fatalf("OnError = %q, want %q", got.Error(), "send: send failed")
	}
	if m.State() != StateDown {
		t.Fatalf("state = %v, want down after threshold", m.State())
	}
}
