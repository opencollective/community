# Email

Email is how people prove ownership of their account and how followers
receive the community's writing. It is configured progressively: the wizard
asks for a provider the first time the server needs to send anything
(step 4, just before verifying the admin's own address).

## Provider interface

One Go interface, implementations in `internal/mail/`:

```go
type Mailer interface {
    Send(ctx context.Context, msg Message) error
    // Verify checks that the configured From domain is allowed to send,
    // returning actionable details (e.g. missing DNS records) if not.
    Verify(ctx context.Context) (*DomainStatus, error)
}

type Message struct {
    To       []string
    Subject  string
    Text     string // always present
    HTML     string // rendered from markdown for newsletters
    ListUnsubscribe string // RFC 8058 one-click unsubscribe, set for newsletters
}
```

Adding a provider = one file implementing `Mailer` plus a row in the
provider dropdown. No other code changes.

### Resend (first implementation)

Plain `net/http` calls to `https://api.resend.com/emails` — no SDK
dependency. `Verify` uses the domains API to check the From domain and
surfaces the exact DNS records (SPF, DKIM) the wizard shows the operator.

## What gets sent

| email | trigger | contains |
|---|---|---|
| Login / verification code | login, setup step 5, follow confirmation, join application | 6-digit code, 10 min expiry |
| Follow confirmation | someone follows with their email | confirm link/code (double opt-in — see [flows/follow.md](../flows/follow.md)) |
| Newsletter | community publishes a kind 30023 blog post | the article, markdown rendered to HTML, with unsubscribe link |
| Application decided | a join application is approved or declined | outcome + login link |
| Thread reply notification | a reply lands on an external participant's thread ([channels](../nostr/channels.md)) | the reply + thread link; batched within a short window |

## Newsletter pipeline

1. communityd maintains a persistent subscription to the local relay for
   kind 30023 events authored by the community npub.
2. On a new event id not present in `newsletter_log`, render markdown → HTML
   (server-side, sanitized), select all identities with a confirmed email and
   newsletter opt-in, and send in batches with retry + backoff.
3. Record progress per recipient batch in `newsletter_log` so a crash
   mid-send resumes without double-sending.

Announcements (kind 1) are **not** emailed — only long-form content is
([flows/follow.md](../flows/follow.md)).

## Deliverability notes

- The From address lives on the community's domain and the wizard refuses to
  finish step 4 until the provider verifies it — this is what keeps the
  newsletter out of spam folders.
- Every newsletter includes `List-Unsubscribe` and a footer unsubscribe link
  that flips the recipient's opt-in flag without requiring login.
