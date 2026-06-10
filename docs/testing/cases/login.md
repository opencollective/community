# Test cases — login and web sessions

Flow reference: [flows/login.md](../../flows/login.md)

### LOGIN-01 — email code logs a member in
Given member `alice` requests a code at `/login`
Then a 6-digit code is emailed, stored only as a hash
And entering it creates a web session cookie (HttpOnly, Secure)
And the code cannot be used a second time

### LOGIN-02 — wrong codes are bounded
Given a valid code was issued
When 5 wrong attempts are made
Then the code is invalidated and a fresh one must be requested

### LOGIN-03 — codes expire
Given a code was issued
When 10 minutes pass (fake clock)
Then the code is rejected as expired

### LOGIN-04 — unknown emails reveal nothing
Given `nobody@example.org` has no account
When a code is requested for it
Then the page response is indistinguishable from LOGIN-01
And no email is sent

### LOGIN-05 — code requests are rate-limited
Given 3 codes were requested for one address within an hour
Then the 4th request is refused with a retry-after message

### LOGIN-06 — logout and log-out-everywhere
Given alice has sessions on two browsers
When she logs out on one, that session alone dies
When she uses "log out everywhere", both die
And her NIP-46 bunker sessions are unaffected by either

### LOGIN-07 — role changes apply to live sessions
Given alice is logged in without the approve-members permission
When the admin grants her the steward role
Then her existing session can immediately act on `/members/pending` (no re-login)
