package store

import (
	"path/filepath"
	"testing"
)

func TestServerRegistryAndCommunityCreation(t *testing.T) {
	s, err := OpenServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if n, _ := s.CommunityCount(); n != 0 {
		t.Fatalf("fresh server must have 0 communities, got %d", n)
	}
	c, err := s.CreateCommunity("hub", "commonshub.brussels")
	if err != nil {
		t.Fatal(err)
	}
	if c.Dir != filepath.Join(s.DataDir, "communities", "hub") {
		t.Fatalf("unexpected community dir %s", c.Dir)
	}
	if err := c.SetSetting("name", "Commons Hub"); err != nil {
		t.Fatal(err)
	}
	if v, err := c.Setting("name"); err != nil || v != "Commons Hub" {
		t.Fatalf("setting round trip: %q %v", v, err)
	}
}

// TestTEN01_HostResolution pins the resolution half of TEN-01.
func TestTEN01_HostResolution(t *testing.T) {
	s, err := OpenServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateCommunity("alpha", "alpha.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCommunity("beta", "beta.test"); err != nil {
		t.Fatal(err)
	}

	a, err := s.CommunityByHost("ALPHA.test:8443")
	if err != nil || a.Slug != "alpha" {
		t.Fatalf("host resolution failed: %v", err)
	}
	b, err := s.CommunityByHost("beta.test")
	if err != nil || b.Slug != "beta" {
		t.Fatalf("host resolution failed: %v", err)
	}
	if _, err := s.CommunityByHost("unknown.test"); err != ErrNotFound {
		t.Fatalf("unknown host must be ErrNotFound, got %v", err)
	}
}

// TestTEN04_NoCrossReferences pins the isolation half of TEN-04: each
// community database is its own file with its own rows.
func TestTEN04_NoCrossReferences(t *testing.T) {
	s, err := OpenServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.CreateCommunity("alpha", "alpha.test")
	b, _ := s.CreateCommunity("beta", "beta.test")
	if err := a.SetSetting("name", "Alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Setting("name"); err != ErrNotFound {
		t.Fatalf("beta must not see alpha's settings, got %v", err)
	}
}
