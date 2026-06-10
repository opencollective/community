# Test cases — setup wizard

Flow reference: [flows/setup.md](../../flows/setup.md)

### SETUP-01 — fresh install lands on the wizard
Given a freshly installed server with an empty database
When anyone opens any path over HTTP
Then they are redirected to `/setup` (step 1)

### SETUP-02 — domain not pointing here is rejected with guidance
Given the operator submits a domain whose A record does not resolve to this server
Then step 1 fails with an error naming the expected record and the server's public IP
And no certificate request is made

### SETUP-03 — valid domain obtains a certificate and switches to HTTPS
Given the operator submits a domain that resolves to this server
Then a certificate is obtained (test mode: simulated)
And the browser is redirected to `https://<domain>/setup/password`
And from now on HTTP requests are redirected to HTTPS

### SETUP-04 — wizard resumes at the first incomplete step
Given setup was interrupted after step 2
When the operator opens any path
Then they are redirected to step 3, with steps 1–2 preserved

### SETUP-05 — weak master password is rejected
Given the operator submits a password below the strength threshold
Then step 2 fails with the strength requirement and nothing is stored

### SETUP-06 — master password creates wrapped keys per unlock mode
Given the operator completes step 2 with strict mode unchecked
Then a DEK exists wrapped by the password-derived KEK and by the machine key
Given strict mode was checked instead
Then no machine-wrapped copy exists

### SETUP-07 — admin username is validated live
Given the operator enters a reserved or malformed username (`admin`, `Aa!`, 1 char)
Then step 3 shows the specific rule violated
And a valid username generates an identity whose nsec is stored encrypted

### SETUP-08 — unverified sending domain blocks step 4 with DNS records
Given a Resend API key whose From-domain is not verified
When the operator saves step 4
Then the exact missing DNS records are displayed and the step does not complete

### SETUP-09 — verified provider sends a real test email
Given a Resend API key with a verified From-domain
When the operator saves step 4
Then a test email is sent through the provider and step 5 begins

### SETUP-10 — admin email verification logs the operator in
Given the operator enters their email on step 5
Then a 6-digit code is emailed
And entering it binds the email to the admin identity and creates a web session
And a wrong code is rejected; 5 wrong attempts invalidate the code

### SETUP-11 — finishing setup creates the community's Nostr presence
When the operator completes step 6 with name, description and icon
Then the community identity exists with a kind 0 profile on the relay
And a kind 34550 community definition is published
And the #general NIP-29 group exists
And default roles steward, moderator, member, follower, fiscal host exist
And the admin holds the steward role and follows the community (kind 3)
And the homepage renders the community profile

### SETUP-12 — the wizard is gone after completion
Given completed setup
When anyone requests any `/setup*` path
Then they get the homepage (or 404), never the wizard
