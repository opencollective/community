# Installation

Target: a fresh Linux VPS (Ubuntu/Debian, amd64 or arm64), a domain you
control, five minutes.

## Requirements

- Ports **80** and **443** reachable from the internet (certificates and all
  traffic; the relay websocket and Blossom share 443).
- An **A/AAAA record** for your domain pointing at the server.
- ~1 GB RAM, a few GB of disk (media grows with use).
- A [Resend](https://resend.com) account (or future provider) able to send
  from your domain.

## What install.sh does

```sh
curl -fsSL https://raw.githubusercontent.com/opencollective/community/main/install.sh | sh
```

1. Detects OS/arch; refuses unsupported platforms with a clear message.
2. Downloads the latest release tarball from GitHub releases — containing
   `communityd` and the pinned `zooid` build — and verifies its sha256
   against the published checksums file.
3. Creates the `community` system user and the
   [file layout](../architecture/storage.md) under `/opt/community`.
4. Installs two systemd units:
   - `zooid.service` — localhost:3334, `Restart=always`, sandboxed
     (`ProtectSystem=strict`, write access only to its data/media dirs);
   - `communityd.service` — binds 80/443 via
     `AmbientCapabilities=CAP_NET_BIND_SERVICE`, same sandboxing.
5. Opens 80/443 in ufw/firewalld when one is active.
6. Starts both services and prints the setup URL:
   `http://<public-ip>/setup`.

Everything after that is the [setup wizard](../flows/setup.md).

Idempotent: re-running upgrades binaries in place and never touches
`data/`, `media/` or `secrets/`.

## Manual control

```sh
systemctl status communityd zooid
journalctl -u communityd -f          # logs
sqlite3 /opt/community/data/app.db   # inspect (secrets are ciphertext)
community --version                  # symlinked to /usr/local/bin
```

## Uninstall

`install.sh --uninstall` stops and removes the units and binaries, and
prints — but never deletes — the data directory. Deleting keys is a human's
job.
