# Documentation

Everything about how Community works and why it is built this way.

## Sections

### [architecture/](architecture)
How the system is put together.

- [overview.md](architecture/overview.md) — the two processes, request routing, tech stack
- [multi-tenancy.md](architecture/multi-tenancy.md) — the fractal model: many communities per server, subgroups as communities, graduation
- [key-management.md](architecture/key-management.md) — envelope encryption, master password, rotation, unlock modes
- [bunker.md](architecture/bunker.md) — the NIP-46 remote signer, sessions, bunker URLs
- [storage.md](architecture/storage.md) — databases, media, file layout
- [email.md](architecture/email.md) — provider interface, Resend, newsletter delivery

### [design/](design)
What it looks like and the principles behind product decisions.

- [principles.md](design/principles.md) — progressive configuration and the other rules we build by
- [design-system.md](design/design-system.md) — tokens, typography, theming (community accent colors)
- [screens.md](design/screens.md) — every route, its purpose and states

### [nostr/](nostr)
The protocol layer.

- [nips.md](nostr/nips.md) — every NIP and event kind we use, and how
- [identities.md](nostr/identities.md) — member and community identities, NIP-05
- [publishing.md](nostr/publishing.md) — posting as the community: NIP-72 proposals and approvals
- [chat.md](nostr/chat.md) — the #general channel as a NIP-29 group
- [channels.md](nostr/channels.md) — the typed channels framework: Proposals, Requests, Events, Expenses
- [money.md](nostr/money.md) — expenses, payments, contributions, fiscal hosts, the treasury

### [flows/](flows)
Step-by-step product flows.

- [setup.md](flows/setup.md) — the six-step wizard
- [follow.md](flows/follow.md) — follower onboarding and the newsletter
- [join.md](flows/join.md) — membership applications and approval
- [login.md](flows/login.md) — email-code login and web sessions
- [roles.md](flows/roles.md) — default roles, permissions, badges

### [testing/](testing)
The behavioral contract, as plain-English test cases.

- [README.md](testing/README.md) — how cases are written, cited by tests, and kept covered
- [environment.md](testing/environment.md) — test environment setup and how to run the suites
- [cases/](testing/cases) — one file per flow: setup, follow, join, login, chat, publishing, newsletter, roles, bunker, keys

### [operations/](operations)
Running a server.

- [install.md](operations/install.md) — requirements and what install.sh does
- [updating.md](operations/updating.md) — releases, the zooid pin, updating safely
- [backup.md](operations/backup.md) — what to back up and how to restore

### [decisions/](decisions)
Architecture decision records (ADRs). Each file captures one significant
decision: its context, the decision itself, and its consequences. ADRs are
immutable once accepted — a reversal gets a new ADR that supersedes the old
one. Numbered in the order they were made.
