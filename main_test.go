package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/taigrr/eff/monitor"
)

func TestBuildConfigAppliesDefaultTarget(t *testing.T) {
	cfg, _, err := buildConfig(nil, options{
		interval:  time.Second,
		threshold: 2,
	})
	if err != nil {
		t.Fatalf("buildConfig() error = %v", err)
	}
	if cfg.Target != "1.1.1.1" {
		t.Fatalf("Target = %q, want 1.1.1.1", cfg.Target)
	}
}

func TestBuildConfigUsesPositionalTarget(t *testing.T) {
	cfg, _, err := buildConfig([]string{"8.8.8.8"}, options{
		interval:  time.Second,
		threshold: 2,
	})
	if err != nil {
		t.Fatalf("buildConfig() error = %v", err)
	}
	if cfg.Target != "8.8.8.8" {
		t.Fatalf("Target = %q, want 8.8.8.8", cfg.Target)
	}
}

func TestBuildConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		opts options
		want string
	}{
		{
			name: "non-positive interval",
			opts: options{interval: 0, threshold: 1},
			want: "interval must be positive",
		},
		{
			name: "threshold below one",
			opts: options{interval: time.Second, threshold: 0},
			want: "threshold must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := buildConfig(nil, tt.opts)
			if err == nil {
				t.Fatal("buildConfig() error = nil, want error")
			}
			if got := err.Error(); !strings.HasPrefix(got, tt.want) {
				t.Fatalf("buildConfig() error = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func TestBuildConfigSetsLoggerLevel(t *testing.T) {
	tests := []struct {
		name      string
		debug     bool
		wantDebug bool
	}{
		{name: "default level", debug: false, wantDebug: false},
		{name: "debug level", debug: true, wantDebug: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, logger, err := buildConfig(nil, options{
				interval:  time.Second,
				threshold: 2,
				debug:     tt.debug,
			})
			if err != nil {
				t.Fatalf("buildConfig() error = %v", err)
			}

			gotDebug := logger.Handler().Enabled(context.Background(), slog.LevelDebug)
			if gotDebug != tt.wantDebug {
				t.Fatalf("debug enabled = %t, want %t", gotDebug, tt.wantDebug)
			}
		})
	}
}

func TestNewRootCmdPassesConfigToRunner(t *testing.T) {
	var got monitor.Config
	cmd := newRootCmd(func(_ context.Context, cfg monitor.Config) error {
		got = cfg
		return nil
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--interval", "5s", "--threshold", "4", "--notify=false", "--privileged", "8.8.4.4"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Target != "8.8.4.4" {
		t.Fatalf("Target = %q, want 8.8.4.4", got.Target)
	}
	if got.Interval != 5*time.Second {
		t.Fatalf("Interval = %s, want 5s", got.Interval)
	}
	if got.Threshold != 4 {
		t.Fatalf("Threshold = %d, want 4", got.Threshold)
	}
	if !got.Privileged {
		t.Fatal("Privileged = false, want true")
	}
}

func TestNewRootCmdReturnsRunnerError(t *testing.T) {
	want := errors.New("boom")
	cmd := newRootCmd(func(_ context.Context, cfg monitor.Config) error {
		_ = cfg
		return want
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"1.1.1.1"})

	if err := cmd.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestMonitorSignalsIncludeInterruptAndTerminate(t *testing.T) {
	signals := monitorSignals()

	if len(signals) != 2 {
		t.Fatalf("len(monitorSignals()) = %d, want 2", len(signals))
	}
	if signals[0] != os.Interrupt {
		t.Fatalf("signals[0] = %v, want %v", signals[0], os.Interrupt)
	}
	if signals[1].String() != "terminated" {
		t.Fatalf("signals[1] = %q, want terminated", signals[1])
	}
}

func TestNotificationCommand(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		title   string
		body    string
		urgency string
		wantNil bool
		want    []string
	}{
		{
			name:    "linux uses notify-send",
			goos:    "linux",
			title:   "eff: Network Down",
			body:    "Network DOWN — 1.1.1.1 unreachable",
			urgency: "critical",
			want:    []string{"notify-send", "-u", "critical", "eff: Network Down", "Network DOWN — 1.1.1.1 unreachable"},
		},
		{
			name:    "darwin uses osascript",
			goos:    "darwin",
			title:   "eff: Network Restored",
			body:    "Network UP — 1.1.1.1 reachable",
			urgency: "normal",
			want:    []string{"osascript", "-e", `display notification "Network UP — 1.1.1.1 reachable" with title "eff: Network Restored"`},
		},
		{
			name:    "unsupported OS returns nil",
			goos:    "windows",
			title:   "ignored",
			body:    "ignored",
			urgency: "normal",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := notificationCommand(tt.goos, tt.title, tt.body, tt.urgency)
			if tt.wantNil {
				if cmd != nil {
					t.Fatalf("notificationCommand() = %v, want nil", cmd.Args)
				}
				return
			}

			if cmd == nil {
				t.Fatal("notificationCommand() = nil, want command")
			}
			if len(cmd.Args) != len(tt.want) {
				t.Fatalf("len(cmd.Args) = %d, want %d (%v)", len(cmd.Args), len(tt.want), cmd.Args)
			}
			for index, want := range tt.want {
				if cmd.Args[index] != want {
					t.Fatalf("cmd.Args[%d] = %q, want %q", index, cmd.Args[index], want)
				}
			}
		})
	}
}
