# Multi-tenancy: the fractal model

Any server can host many collectives, communities and groups. And every
community eventually grows subgroups — circles, working groups, local
chapters. The model that makes both simple is the same model:

> **The community is the atomic unit, and every group is a whole community.**
> A subgroup is not a feature bolted onto its parent — it is a full
> community with its own channels, proposals, requests, expenses, resources,
> announcements, blog, newsletter, roles, members and keys. Communities can
> be members of communities. The structure replicates like a fractal.

This is [design principle 8](../design/principles.md); the decision record is
[ADR 0009](../decisions/0009-fractal-multi-tenancy.md).

## The atomic unit

Everything a community is, it owns exclusively:

| resource | per community |
|---|---|
| hostname | one (sub)domain, e.g. `design.community.example.org` |
| relay | one zooid virtual relay (zooid is multi-tenant by design — one TOML, one schema) |
| database | **one self-contained SQLite file**: settings, identities with encrypted keys, channels, roles, indexes |
| media | one Blossom namespace |
| encryption | one DEK, wrapped by *this community's* admin master password |
| identity | one community npub publishing its kind 0 / 34550 / posts |
| email | its own From address and newsletter |

Nothing in one community's database references another community's rows.

## One server, many communities

- A small **server registry** (`server.db`) maps hostnames to community
  directories and records parent→subgroup links. It holds no community
  content.
- **Routing**: every request resolves Host → community in middleware; the
  resolved community travels in the request context. No handler ever reads
  global state. (This convention applies from the very first PR, even while
  the server hosts exactly one community.)
- **TLS**: autocert's host policy is backed by the registry; each hostname
  gets its certificate on first use.
- Still two processes: one communityd, one zooid — zooid receives one config
  file per community and hot-reloads.

## Subgroups are communities

Creating a working group provisions a community (typically on a subdomain of
the parent) plus two links:

1. **Registry link** — `parent` in `server.db`, used for navigation and
   discovery in the UI;
2. **Protocol link** — the subgroup's community npub is enrolled as a
   *member* of the parent community, exactly as a person would be. A
   collective being part of a collective is the same mechanism as a person
   being part of a collective.

A person active in both parent and subgroup keeps **one keypair**: the same
identity is enrolled in each community's database (each db stays
self-contained — including the encrypted key — so each community remains
independently portable).

Everything behaves identically at every level: a circle of four people has
the same proposals channel, expense template and newsletter machinery as the
root collective. There is no "subgroup mode".

## Graduation

Because a community is self-contained, leaving home is an export, not a
migration project:

1. `community export <slug>` → archive of the community directory (app.db +
   secrets), its relay events (zooid JSONL export for that schema), and its
   media blobs;
2. install Community on the new server, `community import <archive>`;
3. point the (sub)domain at the new server; certificates re-issue on first
   request.

The per-community master password is what makes this trustless-ish: the
departing admin unlocks the DEK on the new server with the password only
they hold. The old server deletes its copy. The reverse move — an
independent community joining a server as a subgroup — is the same import
plus the two parent links.

## v1 scope

v1 ships as a single-community product, but the architecture above is real
from the start: per-community directory layout, db-per-community, Host
resolution middleware, per-community DEK, per-community uniqueness. What's
deferred until the hosted/multi-community milestone: a provisioning flow
("create a community" beyond the setup wizard), custom-domain attachment UX,
wildcard certificates, quotas and abuse controls, and any platform-operator
admin surface ([ADR 0009](../decisions/0009-fractal-multi-tenancy.md)).
