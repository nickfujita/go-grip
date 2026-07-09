# contrib

Optional integration files for running go-grip as a long-lived service.

## `gogrip.service` — systemd **user** unit

Runs `go-grip` headlessly from your home directory, serving markdown with the
built-in `nightshade` theme. Because it is a *user* unit it needs no root and
runs as you.

### Install

```bash
# 1. Make sure the binary is on PATH at the location the unit expects.
#    (See the project README for the release install one-liner.)
ls ~/.local/bin/go-grip

# 2. Drop the unit into your user unit directory.
mkdir -p ~/.config/systemd/user
cp gogrip.service ~/.config/systemd/user/

# 3. Let your user services keep running when you are not logged in.
#    Without this, systemd stops your user units the moment your last
#    session ends (e.g. you disconnect the SSH session that started it).
loginctl enable-linger "$USER"

# 4. Enable + start it.
systemctl --user daemon-reload
systemctl --user enable --now gogrip.service
```

### Check / logs

```bash
systemctl --user status gogrip.service
journalctl --user -u gogrip.service -f
```

### Customize

The unit serves `%h` (your home directory) on go-grip's default port `6419`.
To serve a different directory or port, edit `WorkingDirectory=` and append
`-p <port>` / a path to `ExecStart=`, then `systemctl --user daemon-reload &&
systemctl --user restart gogrip.service`.
