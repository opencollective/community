# 0006 — systemd on a Linux VPS, prebuilt release binaries

Status: accepted (2026-06-10)

## Context

`install.sh` needs a deployment target and a way to obtain binaries.
Considered: systemd units vs Docker compose; prebuilt GitHub release
binaries vs building from source on the server vs container images.

## Decision

- **Target**: bare Linux (Ubuntu/Debian, amd64/arm64) with two sandboxed
  systemd units ([operations/install.md](../operations/install.md)).
- **Distribution**: GitHub releases produced by our CI containing
  `communityd` and the pinned-commit `zooid` build, verified by checksum at
  install time.

## Consequences

- Maximum inspectability: journalctl logs, plain files, plain SQLite —
  aligned with [design principle 4](../design/principles.md).
- No compiler, no Node, no Docker daemon on production machines; installs
  are a download.
- TLS on 80/443 lives in communityd (autocert), so no reverse-proxy
  container choreography.
- Cost: we maintain CI cross-compilation (CGO for SQLite on both arches)
  and a checksum pipeline. A Docker compose variant remains future work for
  operators who prefer containers; nothing in the architecture precludes it.
