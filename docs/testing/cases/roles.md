# Test cases — roles and permissions

Flow reference: [flows/roles.md](../../flows/roles.md)

### ROLE-01 — defaults exist and are protected
Given a completed setup
Then roles steward, moderator, member, follower exist with documented default permissions
And none of them can be deleted (rename and recolor are allowed)

### ROLE-02 — only manage-roles holders manage roles
Given a plain member
Then `/roles` is read-only for them and mutation requests are rejected
Given the admin (or a manage-roles holder)
Then they can create roles and assign/remove members

### ROLE-03 — a custom role works as badge and permission set
Given the admin creates role `founding` with a color and no permissions
And assigns @dan
Then the badge renders next to @dan in chat and the members directory
And @dan gains no new capabilities

### ROLE-04 — granting a permission takes effect immediately
Given @dan has no approve-posts permission and a proposal is pending
When the admin adds @dan to stewards
Then @dan can sign an approval right away (no re-login), and it counts toward quorum

### ROLE-05 — revoking a role revokes its consequences
Given steward @bob
When the role is removed
Then he can no longer approve posts or members (PUB-10 covers in-flight approvals)
And he is removed as moderator from the kind 34550 definition

### ROLE-06 — protocol projection of roles
Given stewards @alice and @bob and moderator @carol
Then the kind 34550 community definition lists alice and bob as moderators
And carol is a NIP-29 group admin on the relay

### ROLE-07 — badge overflow
Given a member holding four roles
Then listings show the first two badges plus "+2", profile shows all
