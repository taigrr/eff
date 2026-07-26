package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/taigrr/eff/monitor"
)

var version = "dev"

type options struct {
	target     string
	interval   time.Duration
	threshold  int
	notify     bool
	debug      bool
	privileged bool
}

func main() {
	cmd := newRootCmd(func(ctx context.Context, cfg monitor.Config) error {
		m := monitor.New(cfg)
		return m.Run(ctx)
	})

	if err := fang.Execute(context.Background(), cmd); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(runMonitor func(context.Context, monitor.Config) error) *cobra.Command {
	opts := options{}

	rootCmd := &cobra.Command{
		Use:   "eff [target]",
		Short: "Network connectivity monitor",
		Long: `Eff continuously pings a target and alerts you when your network goes
down or comes back up. Designed for diagnosing flaky connections.

Sends desktop notifications (via notify-send on Linux, osascript on macOS)
when connectivity state changes.`,
		Version: version,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, logger, err := buildConfig(args, opts)
			if err != nil {
				return err
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			logger.Info("monitoring", "target", cfg.Target, "interval", cfg.Interval, "threshold", cfg.Threshold)
			return runMonitor(ctx, cfg)
		},
	}

	rootCmd.Flags().DurationVarP(&opts.interval, "interval", "i", 3*time.Second, "Ping interval")
	rootCmd.Flags().IntVarP(&opts.threshold, "threshold", "t", 3, "Consecutive failures before declaring down")
	rootCmd.Flags().BoolVarP(&opts.notify, "notify", "n", true, "Send desktop notifications on state changes")
	rootCmd.Flags().BoolVar(&opts.debug, "debug", false, "Show individual ping results")
	rootCmd.Flags().BoolVar(&opts.privileged, "privileged", false, "Use privileged raw ICMP sockets")

	return rootCmd
}

func buildConfig(args []string, opts options) (monitor.Config, *slog.Logger, error) {
	if len(args) > 0 {
		opts.target = args[0]
	}

	level := slog.LevelInfo
	if opts.debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	cfg := monitor.Config{
		Target:     opts.target,
		Interval:   opts.interval,
		Threshold:  opts.threshold,
		Privileged: opts.privileged,
		OnStateChange: func(e monitor.Event) {
			switch e.State {
			case monitor.StateDown:
				msg := fmt.Sprintf("Network DOWN — %s unreachable", e.Target)
				logger.Warn(msg)
				if opts.notify {
					sendNotification("eff: Network Down", msg, "critical")
				}
			case monitor.StateUp:
				msg := fmt.Sprintf("Network UP — %s reachable (was down for %s)",
					e.Target, e.Duration.Round(time.Second))
				logger.Info(msg)
				if opts.notify {
					sendNotification("eff: Network Restored", msg, "normal")
				}
			}
		},
		OnPing: func(seq int, rtt time.Duration) {
			logger.Debug("ping", "seq", seq, "rtt", rtt.Round(time.Microsecond))
		},
		OnError: func(err error) {
			logger.Error("ping error", "error", err)
		},
	}
	if cfg.Interval <= 0 {
		return monitor.Config{}, nil, fmt.Errorf("interval must be positive, got %s", cfg.Interval)
	}
	if cfg.Threshold < 1 {
		return monitor.Config{}, nil, fmt.Errorf("threshold must be at least 1, got %d", cfg.Threshold)
	}
	if err := cfg.Normalize(); err != nil {
		return monitor.Config{}, nil, err
	}

	return cfg, logger, nil
}

func sendNotification(title, body, urgency string) {
	switch runtime.GOOS {
	case "linux":
		exec.Command("notify-send", "-u", urgency, title, body).Run()
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		exec.Command("osascript", "-e", script).Run()
	}
}
