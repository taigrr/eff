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

func main() {
	var (
		target    string
		interval  time.Duration
		threshold int
		notify    bool
		debug     bool
	)

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
			if len(args) > 0 {
				target = args[0]
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			level := slog.LevelInfo
			if debug {
				level = slog.LevelDebug
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

			cfg := monitor.Config{
				Target:    target,
				Interval:  interval,
				Threshold: threshold,
				OnStateChange: func(e monitor.Event) {
					switch e.State {
					case monitor.StateDown:
						msg := fmt.Sprintf("Network DOWN — %s unreachable", e.Target)
						logger.Warn(msg)
						if notify {
							sendNotification("eff: Network Down", msg, "critical")
						}
					case monitor.StateUp:
						msg := fmt.Sprintf("Network UP — %s reachable (was down for %s)",
							e.Target, e.Duration.Round(time.Second))
						logger.Info(msg)
						if notify {
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

			if interval <= 0 {
				return fmt.Errorf("interval must be positive, got %s", interval)
			}
			if threshold < 1 {
				return fmt.Errorf("threshold must be at least 1, got %d", threshold)
			}

			m := monitor.New(cfg)
			logger.Info("monitoring", "target", cfg.Target, "interval", cfg.Interval, "threshold", cfg.Threshold)
			return m.Run(ctx)
		},
	}

	rootCmd.Flags().DurationVarP(&interval, "interval", "i", 3*time.Second, "Ping interval")
	rootCmd.Flags().IntVarP(&threshold, "threshold", "t", 3, "Consecutive failures before declaring down")
	rootCmd.Flags().BoolVarP(&notify, "notify", "n", true, "Send desktop notifications on state changes")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Show individual ping results")

	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		os.Exit(1)
	}
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
