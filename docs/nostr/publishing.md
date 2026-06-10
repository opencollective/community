# Publishing as the community

Nobody — not even the admin's session alone in the web UI — makes the
community speak without the configured approvals. The whole trail is signed
Nostr events on the community's own relay
([decision 0005](../decisions/0005-nip72-for-community-publishing.md)).

## The flow

```
@alice (propose permission)
  └─ signs the draft with HER key
     kind 1 (announcement) or kind 30023 (blog post)
     + ["a", "34550:<community-pubkey>:<community-d>"] tag
        │
        ▼  stored on the local relay (members-only, NIP-42)
@bob, @carol (approve permission)
  └─ each signs a kind 4550 approval (NIP-72)
     embedding the exact proposal JSON
        │
        ▼  communityd watches its relay
policy satisfied?  admin alone, or N distinct stewards (default 2),
                   proposer's own approval never counts
        │
        ▼
bunker signs the final event with the COMMUNITY key
  - same content
  - tags: ["p", <author>, "", "author"], ["e", <proposal-id>, "", "mention"]
  - for 30023: fresh d-tag owned by the community
        │
        ├──► homepage (announcements / blog)
        ├──► email newsletter (kind 30023 only)
        └──► any Nostr client following the community
```

## Why each property holds

- **Provenance is unforgeable.** "Who proposed" is the signature on the
  proposal; "who approved" is the signature on each kind 4550. The app
  database only indexes these for rendering — delete it and the trail
  survives on the relay.
- **Edits reset approvals, automatically.** A kind 4550 embeds the exact
  event it approves. Editing a draft produces a new event id, so prior
  approvals simply don't reference it. Quorum is always over the exact bytes
  to be published.
- **The quorum is enforced at the signer.** The community key lives in the
  bunker; the bunker's only code path for community-key signatures runs the
  policy check first ([architecture/bunker.md](../architecture/bunker.md)).

## Policy

Stored in settings, edited at `/settings/community`:

| option | meaning |
|---|---|
| `admin` | only the admin publishes |
| `admin_or_stewards(n)` | the admin alone, or `n` distinct steward approvals (default, n=2) |
| `any_steward` | one steward approval suffices |

Rules in all modes: the proposer's own approval doesn't count; an approval
from someone who has lost the role between signing and quorum is re-checked
at publish time.

## Declines

NIP-72 doesn't define rejection, so a decline is a NIP-32 label: kind 1985
with `["L", "community.opencollective"]`,
`["l", "declined", "community.opencollective"]`, an `e` tag to the proposal
and a human-readable reason in content — signed by the decliner. A declined
proposal can be revised and resubmitted (new event, fresh approvals).

## Visibility

Proposals and approvals live on the members-only relay: every member can see
what is about to be published in the community's name and by whom. This is
deliberate transparency. (Steward-only drafts would need NIP-59 gift
wrapping; see [nips.md](nips.md) "deliberately not used".)

## Interop

Because proposals carry the NIP-72 `a` tag and approvals are standard kind
4550, NIP-72-aware clients (Coracle, Amethyst, noStrudel) can render the
community, its posts and the approval trail without knowing anything about
this server.
