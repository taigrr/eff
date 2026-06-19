package main

import (
	"context"
	"errors"
	"io"
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

func TestNewRootCmdPassesConfigToRunner(t *testing.T) {
	var got monitor.Config
	cmd := newRootCmd(func(_ context.Context, cfg monitor.Config) error {
		got = cfg
		return nil
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--interval", "5s", "--threshold", "4", "--notify=false", "8.8.4.4"})

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
