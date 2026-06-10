# Updating

## Release model

Our GitHub Actions workflow builds every release:

- `communityd` from this repository;
- `zooid` from [gitea.coracle.social/coracle/zooid](https://gitea.coracle.social/coracle/zooid),
  checked out at the commit pinned in [`ZOOID_VERSION`](../../ZOOID_VERSION)
  at the repo root, built with `CGO_ENABLED=1` for linux amd64 + arm64.

A release = two tarballs (per arch) + `checksums.txt`. Versioning is semver
for our code; the zooid commit is recorded in the release notes.

## Updating a server

```sh
community update         # or re-run install.sh
```

Downloads the latest release, verifies checksums, swaps binaries, restarts
the services. Database migrations run automatically at communityd startup;
zooid migrates its own schema on boot. Config changes are picked up by
zooid's inotify watcher without restarts during normal operation.

Strict-mode servers come back **locked** after an update restart — an admin
must visit `/unlock`. Plan updates accordingly.

## Updating the zooid pin

1. Edit `ZOOID_VERSION` (a commit hash, plus the nostrlib `replace` revision
   if upstream moved it).
2. CI builds and runs the integration smoke test: boot both binaries, run a
   scripted relay/blossom/NIP-29 exchange against the new build.
3. Merge → tag → release. Operators get it with `community update`.

Upstream zooid has no tags yet; pinning by commit is deliberate
([decision 0002](../decisions/0002-zooid-for-relay-and-blossom.md)). If
upstream starts tagging releases, `ZOOID_VERSION` switches to tags with no
other changes.

## Rollback

Releases are immutable; `community update --version vX.Y.Z` installs any
previous release. Database migrations are forward-only — restore from
[backup](backup.md) if a downgrade crosses a migration.
