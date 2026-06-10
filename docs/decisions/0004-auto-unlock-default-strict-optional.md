# 0004 — auto-unlock by default, strict mode opt-in

Status: accepted (2026-06-10)

## Context

If only the master password can unwrap the DEK, a restarted server cannot
sign anything — chat, approvals, login codes, newsletter all stall — until a
human visits `/unlock`. Maximum at-rest security, poor availability for
unattended community servers. The alternative (a machine-held wrap key)
survives restarts but means an attacker with root can recover keys.

## Decision

Both, with availability as the default: a second copy of the DEK wrapped by
`secrets/machine.key` lets the server unlock itself after restarts. A
**strict mode** checkbox (setup step 2, toggleable later with the master
password) deletes the machine-wrapped copy, requiring `/unlock` after every
restart.

## Consequences

- Default installs behave like normal web apps across reboots and updates;
  the database alone (backup leak, snapshot) still reveals no keys.
- Root compromise defeats auto-unlock mode — documented plainly in the
  threat model rather than hidden.
- Strict-mode communities accept paused signing after restarts; the UI keeps
  public pages working and shows members an honest "temporarily locked"
  notice.
- Update tooling must warn strict-mode operators that an update implies an
  unlock visit ([operations/updating.md](../operations/updating.md)).
