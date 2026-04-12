# eff

Network connectivity monitor. Continuously pings a target and sends desktop notifications when your connection drops or recovers.

## Install

```bash
# From source
go install github.com/taigrr/eff@latest

# Or download from releases
```

## Usage

```bash
# Default: ping 1.1.1.1 every 3s, notify on state changes
eff

# Custom target and interval
eff --interval 5s --threshold 5 8.8.8.8

# Disable notifications (log only)
eff --notify=false

# Debug mode (show each ping)
eff --debug
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--interval, -i` | `3s` | Ping interval |
| `--threshold, -t` | `3` | Consecutive failures before declaring down |
| `--notify, -n` | `true` | Send desktop notifications |
| `--debug` | `false` | Show individual ping results |

## How It Works

1. Sends ICMP pings at the configured interval
2. After N consecutive failures (threshold), declares the network **down**
3. Sends a desktop notification (notify-send on Linux, osascript on macOS)
4. When pings succeed again, declares the network **up** and reports downtime duration

## Systemd Service

Run as a persistent user service:

```bash
cp contrib/eff.service ~/.config/systemd/user/
systemctl --user enable --now eff
```

See [contrib/README.md](contrib/README.md) for details.

## Library Usage

The monitor is importable:

```go
import "github.com/taigrr/eff/monitor"

m := monitor.New(monitor.Config{
    Target:    "1.1.1.1",
    Interval:  3 * time.Second,
    Threshold: 3,
    OnStateChange: func(e monitor.Event) {
        fmt.Printf("Network %s\n", e.State)
    },
})

m.Run(ctx)
```

## Background

Built because Xfinity says 500mbps but streams at 0.1mbps. This tool tells you the moment your connection actually drops — no more wondering if it's you or the ISP.
