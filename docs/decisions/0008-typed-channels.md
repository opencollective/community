# 0008 — typed channels framework for Proposals, Requests and future templates

Status: accepted (2026-06-10)

## Context

Beyond #general we need a Proposals channel (members-only discussion threads)
and a Requests channel (externals can post a request to the community), each
toggleable by admins. Already on the roadmap: Expenses (reimbursements),
Resources (things offered to the community) and Products/Services — all
"every message starts a thread, with replies and emoji reactions", differing
only in how a thread starts (form fields) and how the list is browsed.
Building each as a bespoke feature would multiply schemas, routes and
protocol decisions.

## Decision

One framework ([nostr/channels.md](../nostr/channels.md)):

- a **channel registry** in the app database (slug, name, type, template,
  enabled, audiences, position); #general becomes its first row;
- every channel is a **NIP-29 group** on the local relay; two interaction
  shapes: `chat` (kind 9) and `threads` (kind 11 roots, kind 1111 replies,
  kind 7 reactions, NIP-32 labels for lifecycle status);
- a **template** = start-a-thread form + server-side validation of
  structured tags + list renderer + lifecycle labels + audience defaults.
  New channel types are new templates (Go code + a toggle), not new
  architecture;
- **external participation** through lightweight email-verified identities
  (reusing the follower identity machinery and `email_codes`), scoped to the
  public channels they post in, notified of replies by email;
- defaults: Proposals **on**, Requests **off** — exposing a write surface to
  externals is an explicit admin act.

The NIP-72 publishing queue keeps the UI name "Pending posts" to avoid
collision with the Proposals channel.

## Consequences

- Expenses/Resources/Products land later as templates; their structured data
  (amounts, categories, prices) are tags on standard events, so external
  Nostr clients can read them even without our renderers.
- Thread content lives on the relay (rebuildable `threads_index` only in the
  app db), consistent with [principle 3](../design/principles.md).
- Reactions and replies arrive with NIPs 22/25/7D added to our surface —
  all simple, widely-implemented kinds.
- **Open risk**: per-group read scoping in zooid's NIP-29 implementation
  must be validated during implementation. If groups are not read-isolated
  for relay subscribers, fallback options are: communityd-side filtering for
  web rendering (externals only ever read via the web anyway) or upstream
  contribution. Tracked in the implementation milestone.
- Lifecycle status as signed labels means status changes are attributable —
  who approved an expense will be cryptographic record, same as post
  approvals.
