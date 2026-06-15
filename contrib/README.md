# Contrib

## Systemd User Service

Install eff as a systemd user service for persistent monitoring:

```bash
# Copy the service file
cp eff.service ~/.config/systemd/user/eff.service

# Or if installed via .deb/.rpm, it's already at:
# /usr/lib/systemd/user/eff.service

# Enable and start
systemctl --user daemon-reload
systemctl --user enable --now eff

# Check status
systemctl --user status eff

# View logs
journalctl --user -u eff -f
```

### Customizing

Override the target or flags:

```bash
systemctl --user edit eff
```

Add:
```ini
[Service]
ExecStart=
ExecStart=/usr/bin/eff --interval 5s --threshold 5 8.8.8.8
```

### Notifications in user services

The bundled `eff.service` is meant to run as a **user** unit, not a system-wide service.
It sets `DISPLAY`, `DBUS_SESSION_BUS_ADDRESS`, and `XDG_RUNTIME_DIR` so `notify-send` can reach your desktop session.
If notifications still do not appear, confirm the service is running under your user account:

```bash
systemctl --user status eff
systemctl --user show-environment | grep -E 'DISPLAY|DBUS_SESSION_BUS_ADDRESS|XDG_RUNTIME_DIR'
```

### ICMP Permissions

For unprivileged ICMP (no raw sockets), eff uses UDP pings by default.
For raw ICMP (more accurate), either:

1. Run as root (not recommended)
2. Set the capability: `sudo setcap cap_net_raw=+ep $(which eff)`
3. The systemd service sets `AmbientCapabilities=CAP_NET_RAW` automatically
