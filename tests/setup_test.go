//go:build integration

package tests

import (
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/opencollective/community/tests/harness"
)

// TestSETUP01_FreshInstallLandsOnTheWizard pins SETUP-01: with an empty
// database, any path redirects to /setup, which renders step 1.
func TestSETUP01_FreshInstallLandsOnTheWizard(t *testing.T) {
	h := harness.New(t)

	for _, path := range []string{"/", "/members", "/anything/else"} {
		resp := h.Get(path)
		resp.Body.Close()
		if resp.StatusCode != 302 {
			t.Fatalf("GET %s: want 302, got %d", path, resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/setup" {
			t.Fatalf("GET %s: want redirect to /setup, got %q", path, loc)
		}
	}

	resp := h.Get("/setup")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /setup: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Domain or subdomain") {
		t.Fatal("step 1 must ask for the domain")
	}
}

// TestSETUP02_BadDomainRejectedWithGuidance pins the validation half of
// SETUP-02 (the DNS/ACME half exercises CheckDomain in production).
func TestSETUP02_BadDomainRejectedWithGuidance(t *testing.T) {
	h := harness.New(t)

	resp := h.PostForm("/setup", url.Values{"domain": {"not a domain"}})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "does not look like a domain") {
		t.Fatal("invalid domain must show guidance")
	}
	if n, _ := h.Store.CommunityCount(); n != 0 {
		t.Fatal("invalid domain must not create a community")
	}
}
