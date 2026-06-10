# 0009 — fractal multi-tenancy: one database per community, single-community product first

Status: accepted (2026-06-10)

## Context

zooid is already multi-tenant (virtual relays by Host header). The question:
should communityd be multi-tenant from the start, or single-tenant first?
Meanwhile the product vision includes subgroups (circles, working groups)
that must behave exactly like their parent — own proposals, requests,
expenses, resources, newsletter — and should be able to "graduate" to their
own server. Building subgroups as a *feature* of a community and
multi-tenancy as a separate *hosting* concern would mean two parallel
systems for the same idea.

## Decision

One concept ([architecture/multi-tenancy.md](../architecture/multi-tenancy.md)):
the community is the atomic unit; servers host N communities; subgroups
*are* communities (with a registry parent link and their community npub
enrolled as a member of the parent).

Concretely, from the first PR — even while v1 ships as a single-community
product:

- **one self-contained SQLite database per community** under
  `communities/<slug>/`, holding that community's settings, identities
  (including encrypted keys), channels, roles and indexes; a thin
  `server.db` registry maps hostnames and parent links;
- **one DEK and one master password per community**;
- **Host → community resolution middleware**; the community travels in the
  request context; no global settings access anywhere;
- uniqueness (usernames), caches and indexes keyed per community;
- one zooid virtual relay + media namespace per community.

Deferred to the multi-community milestone: provisioning/signup UI,
custom-domain attachment, wildcard certificates, quotas/abuse/billing, and
a platform-operator surface.

## Consequences

- Subgroups and hosted multi-tenancy become the same implementation; "circle
  of four people" and "community on a hosted platform" differ only in who
  runs the box.
- Graduation (and its inverse, joining a server as a subgroup) is an
  export/import of a self-contained directory plus relay JSONL and media —
  not a row-level extraction from a shared database. Per-community DEKs mean
  the departing admin's password unlocks it on the new server.
- Blast-radius containment: a leaked database or cracked password exposes
  one community. (A compromised *process* still exposes all hosted
  communities — inherent to any multi-tenant signer, stated plainly.)
- Costs accepted: cross-community queries (e.g. "everything I'm a member of
  on this server") go through the registry + N databases; the same person's
  key material is stored per community they belong to (re-encrypted under
  each DEK); many open SQLite handles at platform scale (fine for the
  target: tens-to-hundreds of communities per server, WAL mode).
- The single-community v1 carries mild overhead (a registry with one row, a
  resolver with one answer) in exchange for never auditing every query for
  tenant leaks later.
