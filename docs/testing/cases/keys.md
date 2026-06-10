# Test cases — key encryption, rotation, strict mode

Flow reference: [architecture/key-management.md](../../architecture/key-management.md)

### KEY-01 — secrets are ciphertext at rest
Given any populated server
When the raw `app.db` file is scanned
Then no plaintext nsec (hex or bech32) of any identity is present
And non-secret columns (usernames, emails) remain readable with plain sqlite3

### KEY-02 — master password rotation is instant and complete
Given a populated server
When the admin rotates the master password
Then the operation touches only the wrapped-DEK row (no nsec ciphertext changes)
And the old password no longer unlocks; the new one does
And signing continues uninterrupted

### KEY-03 — auto-unlock resumes signing after restart
Given default (auto-unlock) mode
When communityd restarts
Then chat, approvals and login codes work without any human action

### KEY-04 — strict mode locks after restart, /unlock resumes
Given strict mode enabled
When communityd restarts
Then no machine-wrapped DEK exists on disk
And public pages still render; signing actions and code emails show the locked notice
When an admin enters the master password at `/unlock`
Then all paused functions resume
And a wrong password at `/unlock` fails with rate limiting

### KEY-05 — toggling strict mode manages the machine copy
Given auto-unlock mode
When the admin enables strict mode (password required)
Then `secrets/machine.key`-wrapped DEK copy is destroyed
When the admin disables it again
Then a machine-wrapped copy exists and restarts self-unlock

### KEY-06 — ciphertexts are bound to their rows
Given two identities' nsec ciphertexts swapped directly in the database
Then decryption fails loudly for both (AAD binding), and no signature is produced with a mismatched key
