// Package monitor provides continuous network connectivity monitoring
// using ICMP ping. It detects outages and recoveries, tracking statistics
// and invoking callbacks on state changes.
package monitor

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// State represents the current network connectivity state.
type State int

const (
	// StateUnknown is the initial state before any probes.
	StateUnknown State = iota
	// StateUp means the target is reachable.
	StateUp
	// StateDown means the target is unreachable.
	StateDown
)

func (s State) String() string {
	switch s {
	case StateUp:
		return "up"
	case StateDown:
		return "down"
	default:
		return "unknown"
	}
}

// Event is emitted on state changes.
type Event struct {
	State     State
	Target    string
	Timestamp time.Time
	// DownSince is set when State is StateUp, indicating when the outage started.
	DownSince time.Time
	// Duration is set when State is StateUp, indicating how long the outage lasted.
	Duration time.Duration
}

// Stats holds cumulative ping statistics.
type Stats struct {
	PacketsSent int
	PacketsRecv int
	PacketLoss  float64
	MinRTT      time.Duration
	AvgRTT      time.Duration
	MaxRTT      time.Duration
}

// Config configures the monitor.
type Config struct {
	// Target is the hostname or IP to ping (default: "1.1.1.1").
	Target string

	// Interval between pings (default: 3s).
	Interval time.Duration

	// Threshold is the number of consecutive failures before declaring down (default: 3).
	Threshold int

	// Privileged uses raw ICMP sockets (requires root). If false, uses UDP.
	Privileged bool

	// OnStateChange is called when connectivity state changes.
	OnStateChange func(Event)

	// OnPing is called for each successful ping.
	OnPing func(seq int, rtt time.Duration)

	// OnError is called on ping errors (non-timeout).
	OnError func(err error)
}

func (c *Config) defaults() {
	if c.Target == "" {
		c.Target = "1.1.1.1"
	}
	if c.Interval == 0 {
		c.Interval = 3 * time.Second
	}
	if c.Threshold == 0 {
		c.Threshold = 3
	}
}

// Monitor continuously pings a target and reports connectivity changes.
type Monitor struct {
	cfg   Config
	mu    sync.RWMutex
	state State
	stats Stats

	downSince    time.Time
	failureCount int
}

// New creates a new Monitor with the given configuration.
func New(cfg Config) *Monitor {
	cfg.defaults()
	return &Monitor{cfg: cfg, state: StateUnknown}
}

// Run starts the monitor and blocks until the context is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	pinger, err := probing.NewPinger(m.cfg.Target)
	if err != nil {
		return fmt.Errorf("creating pinger for %s: %w", m.cfg.Target, err)
	}

	pinger.SetNetwork("ip4")
	pinger.Size = 56
	pinger.Interval = m.cfg.Interval
	pinger.SetPrivileged(m.cfg.Privileged)

	pinger.OnRecv = func(pkt *probing.Packet) {
		m.handleRecv(pkt)
	}

	pinger.OnFinish = func(stats *probing.Statistics) {
		m.mu.Lock()
		m.stats = Stats{
			PacketsSent: stats.PacketsSent,
			PacketsRecv: stats.PacketsRecv,
			PacketLoss:  stats.PacketLoss,
			MinRTT:      stats.MinRtt,
			AvgRTT:      stats.AvgRtt,
			MaxRTT:      stats.MaxRtt,
		}
		m.mu.Unlock()
	}

	pinger.OnSendError = func(_ *probing.Packet, err error) {
		m.handleFailure()
		if m.cfg.OnError != nil {
			m.cfg.OnError(fmt.Errorf("send: %w", err))
		}
	}

	pinger.OnRecvError = func(err error) {
		if neterr, ok := err.(*net.OpError); ok && neterr.Timeout() {
			m.handleFailure()
			return
		}
		m.handleFailure()
		if m.cfg.OnError != nil {
			m.cfg.OnError(fmt.Errorf("recv: %w", err))
		}
	}

	go func() {
		<-ctx.Done()
		pinger.Stop()
	}()

	return pinger.Run()
}

// State returns the current connectivity state.
func (m *Monitor) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Stats returns cumulative statistics.
func (m *Monitor) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

func (m *Monitor) handleRecv(pkt *probing.Packet) {
	m.mu.Lock()
	wasDown := m.state == StateDown
	downSince := m.downSince

	m.failureCount = 0
	m.state = StateUp
	m.mu.Unlock()

	if m.cfg.OnPing != nil {
		m.cfg.OnPing(pkt.Seq, pkt.Rtt)
	}

	if wasDown && m.cfg.OnStateChange != nil {
		m.cfg.OnStateChange(Event{
			State:     StateUp,
			Target:    m.cfg.Target,
			Timestamp: time.Now(),
			DownSince: downSince,
			Duration:  time.Since(downSince),
		})
	}
}

func (m *Monitor) handleFailure() {
	m.mu.Lock()
	m.failureCount++
	wasUp := m.state != StateDown
	threshold := m.failureCount >= m.cfg.Threshold

	if threshold && wasUp {
		m.state = StateDown
		m.downSince = time.Now()
	}
	m.mu.Unlock()

	if threshold && wasUp && m.cfg.OnStateChange != nil {
		m.cfg.OnStateChange(Event{
			State:     StateDown,
			Target:    m.cfg.Target,
			Timestamp: time.Now(),
		})
	}
}
