//go:build integration

package tests

import (
	"net/url"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/tests/harness"
)

// TestROLE01_DefaultsExistAndAreProtected pins ROLE-01.
func TestROLE01_DefaultsExistAndAreProtected(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	for _, name := range []string{"steward", "moderator", "member", "follower", "fiscal host"} {
		if _, err := c.RoleByName(name); err != nil {
			t.Fatalf("default role %q must exist", name)
		}
	}
	st, _ := c.RoleByName("steward")
	if err := c.DeleteRole(st.ID); err == nil {
		t.Fatal("a default role must not be deletable")
	}
	// Default delete via the UI is refused.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/roles/steward/delete", nil)
	resp.Body.Close()
	if _, err := c.RoleByName("steward"); err != nil {
		t.Fatal("steward must survive a delete attempt")
	}
}

// TestROLE02_OnlyManageRolesHoldersManage pins ROLE-02.
func TestROLE02_OnlyManageRolesHoldersManage(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")

	// A plain member sees the list but cannot create.
	resp, _ := dan.Get(h.Server.URL + "/roles")
	if body(t, resp) == "" {
		t.Fatal("members may view roles")
	}
	resp, _ = dan.PostForm(h.Server.URL+"/roles", url.Values{"name": {"sneaky"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("a plain member must not create roles")
	}
	if _, err := h.Community().RoleByName("sneaky"); err == nil {
		t.Fatal("nothing may have been created")
	}
	// The admin can.
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles", url.Values{"name": {"founding"}})
	resp.Body.Close()
	if _, err := h.Community().RoleByName("founding"); err != nil {
		t.Fatal("the admin must create roles")
	}
}

// TestROLE03_CustomRoleAsBadgeAndPermissions pins ROLE-03.
func TestROLE03_CustomRoleAsBadgeAndPermissions(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan")

	// Create role 'founding' with a color and no permissions, assign dan.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/roles", url.Values{"name": {"founding"}, "color": {"#abc"}})
	resp.Body.Close()
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles/founding/members", url.Values{"username": {"dan"}})
	resp.Body.Close()

	// Badge renders in the members directory.
	page := body(t, mustGet(t, dan, h.Server.URL+"/members"))
	if !strings.Contains(page, "founding") {
		t.Fatal("the custom badge must render next to the member")
	}
	// Dan gains no new capability (no manage_roles).
	resp, _ = dan.PostForm(h.Server.URL+"/roles", url.Values{"name": {"evil"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("a no-permission custom role grants nothing")
	}
}

// TestROLE04_GrantTakesEffectImmediately pins ROLE-04.
func TestROLE04_GrantTakesEffectImmediately(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan") // logged in, no approve_posts
	alice := h.Member("alice", "steward")
	id := propose(t, h, alice, "newsletter", "Two needed", "Needs two approvers.")

	// Dan cannot approve yet.
	resp, _ := dan.PostForm(h.Server.URL+"/posts/pending/"+id+"/approve", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("dan must not approve before the grant")
	}
	// Grant dan steward via the UI (no re-login).
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles/steward/members", url.Values{"username": {"dan"}})
	resp.Body.Close()
	// Now his existing session can act immediately.
	resp, _ = dan.PostForm(h.Server.URL+"/posts/pending/"+id+"/approve", nil)
	resp.Body.Close()
	if resp.StatusCode == 404 {
		t.Fatal("the grant must take effect on the existing session")
	}
	page := body(t, mustGet(t, alice, h.Server.URL+"/posts/pending"))
	if !strings.Contains(page, "@dan") {
		t.Fatal("dan's approval must count after the grant")
	}
}

// TestROLE06_ProtocolProjection pins ROLE-06: stewards are listed as
// moderators in the kind 34550 community definition.
func TestROLE06_ProtocolProjection(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	alice := h.Member("alice", "steward")
	_ = alice

	c := h.Community()
	aliceIdent, _ := c.IdentityByUsername("alice")
	community := communityPub(t, h)

	// Trigger a republish by toggling a steward grant through the UI.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/roles/steward/members", url.Values{"username": {"alice"}})
	resp.Body.Close()

	defs := h.QueryRelayAs("xavier", nostr.Filter{
		Authors: []string{community}, Kinds: []int{publish.KindCommunityDefinition},
	})
	if len(defs) == 0 {
		t.Fatal("no community definition on the relay")
	}
	newest := defs[0]
	for _, d := range defs {
		if d.CreatedAt > newest.CreatedAt {
			newest = d
		}
	}
	found := false
	for _, tag := range newest.Tags.GetAll([]string{"p", ""}) {
		if len(tag) > 1 && tag[1] == aliceIdent.Pubkey {
			found = true
		}
	}
	if !found {
		t.Fatal("the steward must be listed as a moderator in kind 34550")
	}
}

// TestROLE07_BadgeOverflow pins ROLE-07.
func TestROLE07_BadgeOverflow(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	dan := h.Member("dan", "steward", "moderator", "fiscal host")
	_ = dan

	c := h.Community()
	danIdent, _ := c.IdentityByUsername("dan")
	// Add a fourth custom role for a clear overflow.
	resp, _ := h.Admin.PostForm(h.Server.URL+"/roles", url.Values{"name": {"founding"}})
	resp.Body.Close()
	resp, _ = h.Admin.PostForm(h.Server.URL+"/roles/founding/members", url.Values{"username": {"dan"}})
	resp.Body.Close()
	_ = danIdent

	page := body(t, mustGet(t, dan, h.Server.URL+"/members"))
	if !strings.Contains(page, "+") {
		t.Fatal("a member with many roles must show a +n overflow badge")
	}
}
