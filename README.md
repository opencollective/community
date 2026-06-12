# Community

A self-hosted community server, in one `install.sh`.

Community gives any group its own home on the open web: a homepage, a member
directory, a Discord-style channel, a blog with an email newsletter — all built
on [Nostr](https://nostr.com). Every member gets a real, portable Nostr
identity whose key never leaves the server: the server is a
[bunker](docs/architecture/bunker.md) (NIP-46 remote signer), so members can
use any Nostr app without ever touching a private key.

One domain runs everything:

- **Web app** — homepage, join/follow flows, members, roles, publishing
- **Nostr relay + Blossom media server** — powered by [zooid](https://gitea.coracle.social/coracle/zooid)
- **Bunker** — remote signing for members and for the community itself
- **Email** — login codes and a newsletter for followers, via pluggable providers (Resend first)

## Status

Pre-alpha, under active development. The docs were written first and are
the contract — start with [docs/README.md](docs/README.md). Implemented so
far: the storage layout (one SQLite per community, ADR 0009), envelope key
encryption (ADR 0003/0004), identity generation, tenant resolution, and
step 1 of the setup wizard.

## Development

Requires Go 1.26+ and gcc (CGO for SQLite).

```sh
make test       # unit tests
make test-it    # integration tests (httptest harness)
make test-all   # both + case-coverage check against docs/testing/cases
make run        # build and serve on :8080 with ./data
```

Tests cite the plain-English cases in [docs/testing/cases](docs/testing/cases)
by ID (e.g. `TestSETUP01_…`); `scripts/check-case-coverage.sh` keeps the
mapping honest.

## Quick start (target experience)

```sh
curl -fsSL https://raw.githubusercontent.com/opencollective/community/main/install.sh | sh
```

Then open the printed URL and follow the setup wizard: domain → certificate →
master password → your account → email → community profile. Five minutes to a
live community.

## Documentation

| Section | Contents |
|---|---|
| [docs/architecture](docs/architecture) | Components, key management, bunker, storage, email |
| [docs/design](docs/design) | Principles, design system, screens |
| [docs/nostr](docs/nostr) | NIPs used, identities, publishing, chat |
| [docs/flows](docs/flows) | Setup, follow, join, login, roles |
| [docs/operations](docs/operations) | Install, updates, backup |
| [docs/decisions](docs/decisions) | Architecture decision records |

## License

MIT
