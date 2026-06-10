# Publishing as the community

Nobody — not even the admin's session alone in the web UI — makes the
community speak without the configured approvals. The whole trail is signed
Nostr events on the community's own relay
([decision 0005](../decisions/0005-nip72-for-community-publishing.md)).

## Content types

`/compose` offers three types, each with its **own approval policy**
([decision 0011](../decisions/0011-content-governance-rss-log.md)):

| type | kind | published to | emailed |
|---|---|---|---|
| announcement | 1 | homepage — public or members-only, chosen at compose ([channels.md](channels.md) § thread visibility) | no |
| blog post | 30023 | homepage blog, `/posts/{slug}`, **RSS** (`/feed.xml`) | no |
| newsletter | 30023 + a `newsletter` self-label (NIP-32 `l`/`L` tags on the event) | site archive + **email** to subscribed followers and members | yes |

The difference between a blog post and a newsletter is exactly one thing:
the newsletter is sent by email. Blog posts are followed via RSS (or any
Nostr client); the newsletter reaches inboxes.

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
        ├──► homepage (announcements / blog) + RSS (blog)
        ├──► email (newsletter type only)
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

## Policies

Every publishing section has its own policy, in the same shape as channel
policies ([decision 0010](../decisions/0010-channel-approvals-and-events.md)):
a set of approver role(s) plus a required count, edited at
`/settings/community`. The admin alone is always sufficient; setting the
approver roles to none makes a section admin-only.

| section | default policy |
|---|---|
| announcements | 1 steward |
| blog posts | 1 steward |
| newsletter | **2 stewards** — it reaches inboxes, the highest-blast-radius act |
| profile edits | 2 stewards |

Rules in all policies: the proposer's own approval doesn't count; an
approval from someone who has lost the role between signing and quorum is
re-checked at publish time.

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

## Profile edits (the linktree)

The community profile — name, description, icon, and an array of links
(website, socials, budget page, …) rendered as the homepage linktree — is
edited through this same machinery, with one difference in *who may propose*:
**any member** can submit a profile edit request (no propose permission
needed); approval follows the same policy and `approve_posts` permission.

One protocol subtlety: kind 0 is a *replaceable* event, so a proposal must
not be a bare kind 0 signed by the proposer — that would overwrite the
proposer's own profile. Instead the proposal is a **wrapper**: a kind 30078
(NIP-78 app data) event signed by the proposer, with the complete proposed
profile JSON as content and tags `["k","0"]` + the community `a` tag.
Approvals (kind 4550) and declines (kind 1985) reference the wrapper exactly
as they do posts; on quorum the bunker constructs the new community kind 0
from the approved JSON and signs it. The same wrapper pattern applies to any
future proposal targeting a replaceable event.

Rules:
- editable fields are whitelisted: `name`, `about`, `picture`,
  `links` (array of `{label, url}`, http(s) only, capped); everything else
  in the community's kind 0 is server-managed;
- the UI shows a field-level diff against the current profile, and pending
  edits are flagged as stale if the profile changes under them (approvals
  bind to exact content regardless);
- publishing a profile edit does **not** trigger the newsletter.

The `links` array lives in the kind 0 content — a pragmatic extension that
NIP-39-aware clients ignore gracefully; platform identities that fit NIP-39
(`i` tags) can be added alongside later.

## Interop

Because proposals carry the NIP-72 `a` tag and approvals are standard kind
4550, NIP-72-aware clients (Coracle, Amethyst, noStrudel) can render the
community, its posts and the approval trail without knowing anything about
this server.
