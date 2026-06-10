# Test cases — newsletter and transactional email

Flow reference: [architecture/email.md](../../architecture/email.md).
All cases assert against the captured messages of the fake mailer.

### MAIL-01 — a published newsletter is emailed to opted-in recipients
(updated by ADR 0011: the trigger is the newsletter type, not blog posts)
Given followers and members with confirmed emails and newsletter opt-in
When the community publishes a **newsletter** (PUB-13)
Then each opted-in recipient receives exactly one email
And recipients without opt-in, without confirmed email, or unsubscribed receive none

### MAIL-02 — the newsletter has correct text and HTML parts
Given a newsletter written in markdown (headings, links, bold, a list, an image)
Then the email contains a plain-text part with readable markdown-derived text
And an HTML part with the rendered markdown (sanitized: scripts/iframes stripped)
And both parts contain a link to `/posts/{slug}` and an unsubscribe link
And the subject is the post title; the From is the configured community address

### MAIL-03 — one-click unsubscribe headers are present
Given any newsletter email
Then it carries `List-Unsubscribe` and `List-Unsubscribe-Post` headers (RFC 8058)
And the unsubscribe endpoint works without login (FOLLOW-06)

### MAIL-04 — announcements and blog posts are never emailed
(updated by ADR 0011)
Given a published announcement (PUB-04) and a published blog post (PUB-09)
Then no email is sent for either

### MAIL-05 — sending is at-most-once
Given a newsletter that was already sent
When communityd restarts or the relay redelivers the event
Then no recipient receives a duplicate

### MAIL-06 — interrupted sends resume without duplicates
Given 100 opted-in recipients and a crash after 40 sends (fault injection)
When communityd restarts
Then the remaining 60 receive the email and the first 40 get no duplicate

### MAIL-07 — provider failures retry with backoff
Given the mailer is scripted to fail twice then succeed
Then the message is delivered after retries
And persistent failures are surfaced in the admin UI, not silently dropped

### MAIL-08 — transactional emails always flow regardless of opt-in
Given an unsubscribed member
Then login codes and application decisions still reach them
(opt-in governs the newsletter only)
