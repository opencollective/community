//go:build integration

package tests

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/tests/harness"
)

// TestSETUP04_WizardResumesAtFirstIncompleteStep pins SETUP-04.
func TestSETUP04_WizardResumesAtFirstIncompleteStep(t *testing.T) {
	h := harness.New(t)
	client := h.Client()

	post := func(path string, v url.Values) {
		resp, err := client.PostForm(h.Server.URL+path, v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	post("/setup", url.Values{"domain": {harness.Domain}})
	post("/setup/password", url.Values{
		"password": {harness.MasterPassword}, "confirm": {harness.MasterPassword},
	})

	// Steps 1–2 done; any path must land on step 3.
	for _, path := range []string{"/", "/setup", "/setup/email", "/members"} {
		resp, err := client.Get(h.Server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "/setup/admin" {
			t.Fatalf("GET %s: want resume at /setup/admin, got %d %q", path, resp.StatusCode, loc)
		}
	}
}

// TestSETUP05_WeakPasswordRejected pins SETUP-05.
func TestSETUP05_WeakPasswordRejected(t *testing.T) {
	h := harness.New(t)
	client := h.Client()
	resp, _ := client.PostForm(h.Server.URL+"/setup", url.Values{"domain": {harness.Domain}})
	resp.Body.Close()

	resp, err := client.PostForm(h.Server.URL+"/setup/password",
		url.Values{"password": {"short"}, "confirm": {"short"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "at least 12 characters") {
		t.Fatal("weak password must be rejected with the requirement")
	}
	if _, err := h.Community().Setting("wrapped_dek"); err == nil {
		t.Fatal("weak password must store nothing")
	}
}

// TestSETUP06_KeyWrappingPerUnlockMode pins SETUP-06.
func TestSETUP06_KeyWrappingPerUnlockMode(t *testing.T) {
	for _, strict := range []bool{false, true} {
		h := harness.New(t)
		h.CompleteSetup(strict)
		c := h.Community()

		if _, err := c.Setting("wrapped_dek"); err != nil {
			t.Fatal("password-wrapped DEK must exist")
		}
		machinePath := filepath.Join(c.Dir, "secrets", "machine.key")
		_, mwErr := c.Setting("machine_wrapped_dek")
		_, fileErr := os.Stat(machinePath)
		if strict {
			if mwErr == nil || fileErr == nil {
				t.Fatal("strict mode must leave no machine-wrapped copy (KEY-04 precondition)")
			}
		} else {
			if mwErr != nil || fileErr != nil {
				t.Fatalf("auto-unlock mode must persist the machine wrap: %v %v", mwErr, fileErr)
			}
		}
	}
}

// TestSETUP08_UnverifiedSendingDomainBlocks pins SETUP-08.
func TestSETUP08_UnverifiedSendingDomainBlocks(t *testing.T) {
	h := harness.New(t)
	h.Mailer.DomainVerified = false
	h.Mailer.MissingRecords = []mail.DNSRecord{
		{Type: "TXT", Name: "resend._domainkey", Value: "p=MIGfMA0..."},
	}

	client := h.Client()
	steps := []struct {
		path string
		v    url.Values
	}{
		{"/setup", url.Values{"domain": {harness.Domain}}},
		{"/setup/password", url.Values{"password": {harness.MasterPassword}, "confirm": {harness.MasterPassword}}},
		{"/setup/admin", url.Values{"username": {"xavier"}}},
	}
	for _, s := range steps {
		resp, err := client.PostForm(h.Server.URL+s.path, s.v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	resp, err := client.PostForm(h.Server.URL+"/setup/email", url.Values{
		"provider": {"resend"}, "api_key": {"re_test"}, "from": {"community@" + harness.Domain},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "resend._domainkey") {
		t.Fatal("unverified domain must display the missing DNS records")
	}

	// The step did not complete: still on step 4.
	resp2, _ := client.Get(h.Server.URL + "/")
	resp2.Body.Close()
	if loc := resp2.Header.Get("Location"); loc != "/setup/email" {
		t.Fatalf("step 4 must not complete while unverified, resumed at %q", loc)
	}
}

// TestSETUP10_CodeVerificationLogsTheAdminIn pins SETUP-10 (full flow via
// CompleteSetup) plus the wrong-code path.
func TestSETUP10_CodeVerificationLogsTheAdminIn(t *testing.T) {
	h := harness.New(t)
	client := h.CompleteSetup(false)

	// Logged in: homepage renders (no wizard redirect), session cookie set.
	resp, err := client.Get(h.Server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("homepage after setup: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("Commons Hub")) {
		t.Fatal("homepage must show the community name")
	}

	admin, err := h.Community().IdentityByEmail(harness.AdminEmail)
	if err != nil || admin.Status != "active" {
		t.Fatalf("admin identity must be active with bound email: %v", err)
	}
}

// TestSETUP10_WrongCodesAreBounded pins the attempt limit (also LOGIN-02's
// mechanism).
func TestSETUP10_WrongCodesAreBounded(t *testing.T) {
	h := harness.New(t)
	client := h.Client()
	steps := []struct {
		path string
		v    url.Values
	}{
		{"/setup", url.Values{"domain": {harness.Domain}}},
		{"/setup/password", url.Values{"password": {harness.MasterPassword}, "confirm": {harness.MasterPassword}}},
		{"/setup/admin", url.Values{"username": {"xavier"}}},
		{"/setup/email", url.Values{"provider": {"resend"}, "api_key": {"re_test"}, "from": {"community@" + harness.Domain}}},
		{"/setup/verify", url.Values{"email": {harness.AdminEmail}}},
	}
	for _, s := range steps {
		resp, err := client.PostForm(h.Server.URL+s.path, s.v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	code := h.Mailer.LastCodeTo(harness.AdminEmail)

	for i := 0; i < 5; i++ {
		resp, _ := client.PostForm(h.Server.URL+"/setup/verify",
			url.Values{"email": {harness.AdminEmail}, "code": {"000000"}})
		resp.Body.Close()
	}
	// 5 wrong attempts invalidate the code: the right one now fails.
	resp, _ := client.PostForm(h.Server.URL+"/setup/verify",
		url.Values{"email": {harness.AdminEmail}, "code": {code}})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "wrong or expired") {
		t.Fatal("the 5th wrong attempt must invalidate the code")
	}
}

// TestSETUP11_FinishCreatesRolesAndIdentities pins the local half of
// SETUP-11 (the relay half lands with the zooid milestone).
func TestSETUP11_FinishCreatesRolesAndIdentities(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	c := h.Community()

	admin, err := c.IdentityByUsername("xavier")
	if err != nil {
		t.Fatal(err)
	}
	roles, err := c.RoleNames(admin.ID)
	if err != nil || len(roles) != 1 || roles[0] != "steward" {
		t.Fatalf("admin must hold steward, got %v (%v)", roles, err)
	}
	for _, name := range []string{"steward", "moderator", "member", "follower", "fiscal host"} {
		var n int
		if err := c.DB.QueryRow(`SELECT COUNT(*) FROM roles WHERE name = ? AND is_default = 1`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("default role %q missing", name)
		}
	}
	if _, err := c.IdentityByUsername("community"); err != nil {
		t.Fatal("community identity must exist")
	}
}

// TestSETUP12_WizardGoneAfterCompletion pins SETUP-12, fully this time.
func TestSETUP12_WizardGoneAfterCompletion(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	for _, path := range []string{"/setup", "/setup/password", "/setup/verify", "/setup/community"} {
		resp := h.Get(path)
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("GET %s after setup: want 404, got %d", path, resp.StatusCode)
		}
	}
}

// TestLOGIN03_CodesExpire pins the expiry mechanism via the fake clock.
func TestLOGIN03_CodesExpire(t *testing.T) {
	h := harness.New(t)
	client := h.Client()
	steps := []struct {
		path string
		v    url.Values
	}{
		{"/setup", url.Values{"domain": {harness.Domain}}},
		{"/setup/password", url.Values{"password": {harness.MasterPassword}, "confirm": {harness.MasterPassword}}},
		{"/setup/admin", url.Values{"username": {"xavier"}}},
		{"/setup/email", url.Values{"provider": {"resend"}, "api_key": {"re_test"}, "from": {"community@" + harness.Domain}}},
		{"/setup/verify", url.Values{"email": {harness.AdminEmail}}},
	}
	for _, s := range steps {
		resp, err := client.PostForm(h.Server.URL+s.path, s.v)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	code := h.Mailer.LastCodeTo(harness.AdminEmail)

	h.Clock.Advance(11 * time.Minute)
	resp, _ := client.PostForm(h.Server.URL+"/setup/verify",
		url.Values{"email": {harness.AdminEmail}, "code": {code}})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "wrong or expired") {
		t.Fatal("a code older than 10 minutes must be rejected")
	}
}
