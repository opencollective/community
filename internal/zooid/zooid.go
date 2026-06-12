// Package zooid manages the companion relay+Blossom process
// (docs/architecture/overview.md, ADR 0002): communityd writes one TOML
// config per community into zooid's CONFIG directory (hot-reloaded via
// inotify) and reverse-proxies to its address. zooid itself runs under
// systemd in production and is spawned by the harness in tests.
package zooid

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/store"
)

// Manager points communityd at a zooid instance.
type Manager struct {
	// ConfigDir is zooid's CONFIG directory.
	ConfigDir string
	// Addr is zooid's listen address (host:port), e.g. 127.0.0.1:3334.
	Addr string
}

// HostFor is the Host header value used between communityd and zooid for
// one community. The proxy always rewrites to it, so external Host
// variations (ports, casing) never reach zooid's exact-match dispatch.
func HostFor(c *store.Community) string {
	return c.Hostname
}

var secretRe = regexp.MustCompile(`(?m)^secret\s*=\s*"([0-9a-f]{64})"`)

// WriteConfig writes (or refreshes) the community's virtual-relay config.
// The relay's own secret key is preserved across rewrites. The community
// pubkey gets a can_invite/can_manage role so communityd can fetch invite
// claims and administer the relay.
func (m *Manager) WriteConfig(c *store.Community, communityPubkey, name, description, icon string) error {
	path := filepath.Join(m.ConfigDir, c.Slug+".toml")

	secret := ""
	if existing, err := os.ReadFile(path); err == nil {
		if match := secretRe.FindSubmatch(existing); match != nil {
			secret = string(match[1])
		}
	}
	if secret == "" {
		kp, err := identity.Generate()
		if err != nil {
			return err
		}
		// The relay key signs relay-housekeeping events (invites,
		// membership lists) — it is infrastructure, not an identity, and
		// zooid requires it in its config file.
		secret = kp.SecretHex
	}

	cfg := fmt.Sprintf(`host = %q
schema = %q
secret = %q

[info]
name = %q
icon = %q
pubkey = %q
description = %q

[policy]
public_join = false
strip_signatures = false

[groups]
enabled = true

[management]
enabled = false

[push]
enabled = false

[blossom]
enabled = true
authenticated_read = false
adapter = "local"

[roles.system]
pubkeys = [%q]
can_invite = true
can_manage = true
`, HostFor(c), schemaFor(c), secret, name, icon, communityPubkey, description, communityPubkey)

	if err := os.MkdirAll(m.ConfigDir, 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(cfg), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var nonSchema = regexp.MustCompile(`[^a-z0-9_]`)

func schemaFor(c *store.Community) string {
	s := nonSchema.ReplaceAllString(c.Slug, "_")
	if s == "" {
		s = "main"
	}
	return s
}
