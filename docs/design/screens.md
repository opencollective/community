# Screens

Every route, who can see it, and its states. Wireframes and high-fidelity
mockups were reviewed in design sessions; this file is the canonical list.

## Setup wizard (operator, first run only)

Served over HTTP until step 1 completes; resumes at the first incomplete step.

| route | step | asks | notes |
|---|---|---|---|
| `/setup` | 1 | domain or subdomain | checks DNS points here, obtains certificate, redirects to https |
| `/setup/password` | 2 | master password ×2, strict-mode checkbox | strength meter; "keep it in your keychain" |
| `/setup/admin` | 3 | username, ownership-proof method | generates the admin identity; shows future NIP-05 |
| `/setup/email` | 4 | provider, API key, From address | outgoing only; verifies domain with provider, shows missing DNS records |
| `/setup/verify` | 5 | the admin's own email, then 6-digit code | first real email sent; ends logged in |
| `/setup/community` | 6 | name, description, icon (URL or upload) | creates the community identity + default roles; finish → homepage |

## Public

| route | purpose | states |
|---|---|---|
| `/` | homepage: identity + linktree, follow/join/login, announcements, upcoming events (when enabled and non-empty), blog, channel tabs | visitor vs. member; visitors see locked tabs for member channels, open tabs for enabled public channels |
| `/posts/{slug}` | one blog post | public |
| `/channels/requests` | public thread channel (when enabled): request list and threads | replies/reactions require an identity |
| `/channels/requests/new` | external request form: name, email, text → email code verification | unverified submissions never post; posted requests are pending until approved |
| `/channels/events/public.ics` | subscribable ICS feed of approved **public** events | public |
| `/channels/events/members.ics` | ICS feed of all approved events | requires `?token=` (per-member secret, regenerable) |
| `/feed.xml` | RSS feed of published blog posts | public |
| `/follow` | email-only follow | success = "check your inbox to confirm" |
| `/join` | application form: name, username (live availability), email, motivation, newsletter opt-in | duplicate username/email errors inline |
| `/login` | email → 6-digit code | unknown email gets the same UX (no account enumeration) |
| `/.well-known/nostr.json` | NIP-05 | `?name=` filtered |

## Members only

| route | purpose | states |
|---|---|---|
| `/` (logged in) | announcements + blog + channel tabs (#general chat, thread channels) | chat input disabled for muted members |
| `/channels/{slug}` | thread channel list, rendered by the channel's template, with a status filter (all / pending / approved) | visitors see approved + public threads only; members see all |
| `/channels/{slug}/new` | start a thread, form fields from the template, visibility selector when the channel allows override | needs the channel's post audience |
| `/channels/{slug}/{thread}` | thread view: root, replies, emoji reactions | reply/react per channel audience |
| `/members` | searchable directory with role badges | live filter |
| `/members/{username}` | profile: npub, badges, joined date | |
| `/members/pending` | applications with motivation, approve/decline, vote progress | needs `approve_members` permission to act; visible to all members |
| `/compose` | propose an announcement, blog post or newsletter | needs `propose_posts`; markdown editor with preview; each type has its own approval policy |
| `/profile/edit` | propose a community profile change: name, description, icon, links (add/remove/reorder) | any member; field-level diff in the pending queue |
| `/posts/pending` | pending-posts queue with signed-approval trail: posts and profile edits | approve needs `approve_posts`; proposer cannot approve own |
| `/roles` | role list with member counts | manage needs `manage_roles` |
| `/roles/{role}` | permissions toggles, member chips, rename/recolor | default roles not deletable |
| `/settings/apps` | bunker URL generation, active sessions, revoke; members ICS feed token (regenerate) | per-identity |
| `/settings/community` | profile, theme colors, posting policy, channel toggles + per-channel approval policies (roles, required count), email provider, master password rotation, strict mode | admin only |
| `/log` | activity log: every signed event rendered as a human-readable line, with a raw-JSON inspector per entry, category/author filters, pagination | **admin only** |
| `/unlock` | master password prompt | exists only in strict mode after a restart; non-admins see a "temporarily locked" notice |

## Email-borne

| message | links to |
|---|---|
| login / verification code | (code entry already open) |
| follow confirmation | `/follow/confirm?token=…` |
| newsletter | `/posts/{slug}`, unsubscribe link |
| application decision | `/login` |

## Global states

- **Locked (strict mode, post-restart)** — public pages render normally from
  the database; anything requiring a signature or sending email shows the
  locked notice.
- **Setup incomplete** — every route redirects to the current wizard step.
- **Errors** — inline next to the field where possible; full-page only for
  404/500, in the same visual language.
