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
| `/` | homepage: identity, follow/join/login, announcements, blog, channel tabs | visitor vs. member; visitors see locked tabs for member channels, open tabs for enabled public channels |
| `/posts/{slug}` | one blog post | public |
| `/channels/requests` | public thread channel (when enabled): request list and threads | replies/reactions require an identity |
| `/channels/requests/new` | external request form: name, email, text → email code verification | unverified submissions never post |
| `/follow` | email-only follow | success = "check your inbox to confirm" |
| `/join` | application form: name, username (live availability), email, motivation, newsletter opt-in | duplicate username/email errors inline |
| `/login` | email → 6-digit code | unknown email gets the same UX (no account enumeration) |
| `/.well-known/nostr.json` | NIP-05 | `?name=` filtered |

## Members only

| route | purpose | states |
|---|---|---|
| `/` (logged in) | announcements + blog + channel tabs (#general chat, thread channels) | chat input disabled for muted members |
| `/channels/{slug}` | thread channel list, rendered by the channel's template | members-only or public per channel config |
| `/channels/{slug}/new` | start a thread, form fields from the template | needs the channel's post audience |
| `/channels/{slug}/{thread}` | thread view: root, replies, emoji reactions | reply/react per channel audience |
| `/members` | searchable directory with role badges | live filter |
| `/members/{username}` | profile: npub, badges, joined date | |
| `/members/pending` | applications with motivation, approve/decline, vote progress | needs `approve_members` permission to act; visible to all members |
| `/compose` | propose announcement (kind 1) or blog post (kind 30023) | needs `propose_posts`; markdown editor with preview |
| `/posts/pending` | proposal queue with signed-approval trail | approve needs `approve_posts`; proposer cannot approve own |
| `/roles` | role list with member counts | manage needs `manage_roles` |
| `/roles/{role}` | permissions toggles, member chips, rename/recolor | default roles not deletable |
| `/settings/apps` | bunker URL generation, active sessions, revoke | per-identity |
| `/settings/community` | profile, theme colors, posting policy, channel toggles, email provider, master password rotation, strict mode | admin only |
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
