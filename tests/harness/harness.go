// Package harness boots an isolated communityd for integration tests
// (docs/testing/environment.md): real HTTP stack and SQLite, fake mailer,
// fake clock, fast argon2. Fixtures go through the front door.
package harness

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/bunker"
	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/web"
	"github.com/opencollective/community/internal/zooid"
)

// Domain is the hostname the harness community lives on. httptest binds
// 127.0.0.1, so Host headers resolve to it naturally.
const Domain = "127.0.0.1"

// MasterPassword used by CompleteSetup.
const MasterPassword = "correct horse battery staple"

// AdminEmail used by CompleteSetup.
const AdminEmail = "admin@example.org"

// H is one isolated server instance backed by a temp directory.
type H struct {
	T       *testing.T
	DataDir string
	Store   *store.Server
	App     *web.App
	Server  *httptest.Server
	Mailer  *FakeMailer
	Clock   *Clock
	// Admin is the logged-in admin client, set by CompleteSetup.
	Admin *http.Client
	// RelayURL is the NIP-46 transport relay (communityd's embedded
	// /bunker).
	RelayURL string
	// HasZooid reports whether the real zooid binary is running (bin/zooid
	// from `make zooid`). Tests asserting relay-side state skip without it.
	HasZooid bool

	port     string // stable communityd port across Restart()
	zooidCmd *exec.Cmd
	zooidDir string
}

// New boots a fresh server with an empty data directory, plus the real
// zooid binary when available (`make zooid`).
func New(t *testing.T) *H {
	t.Helper()
	h := &H{
		T:        t,
		DataDir:  t.TempDir(),
		zooidDir: t.TempDir(),
		Mailer:   NewFakeMailer(),
		Clock:    &Clock{t: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)},
	}
	h.port = freePort(t)
	h.startZooid()
	h.boot()
	return h
}

// repoRoot walks up from the test binary's source dir to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}

// startZooid spawns the pinned zooid (one per harness; it outlives
// communityd restarts like the separate process it is in production).
func (h *H) startZooid() {
	bin := filepath.Join(repoRoot(h.T), "bin", "zooid")
	if _, err := os.Stat(bin); err != nil {
		return // run `make zooid` for relay-side coverage
	}
	zooidPort := freePort(h.T)
	for _, sub := range []string{"config", "data", "media"} {
		if err := os.MkdirAll(filepath.Join(h.zooidDir, sub), 0o750); err != nil {
			h.T.Fatal(err)
		}
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"PORT="+zooidPort,
		"CONFIG="+filepath.Join(h.zooidDir, "config"),
		"DATA="+filepath.Join(h.zooidDir, "data"),
		"MEDIA="+filepath.Join(h.zooidDir, "media"),
	)
	if err := cmd.Start(); err != nil {
		h.T.Fatalf("start zooid: %v", err)
	}
	h.zooidCmd = cmd
	h.T.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	// Wait for the listener.
	addr := "127.0.0.1:" + zooidPort
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			h.HasZooid = true
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.T.Fatal("zooid did not start listening")
}

func (h *H) zooidAddr() string {
	if h.zooidCmd == nil {
		return ""
	}
	for _, kv := range h.zooidCmd.Env {
		if strings.HasPrefix(kv, "PORT=") {
			return "127.0.0.1:" + strings.TrimPrefix(kv, "PORT=")
		}
	}
	return ""
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func (h *H) boot() {
	h.T.Helper()
	s, err := store.OpenServer(h.DataDir)
	if err != nil {
		h.T.Fatal(err)
	}
	app, err := web.New(s, slog.New(slog.DiscardHandler))
	if err != nil {
		h.T.Fatal(err)
	}
	app.Argon2 = crypto.TestArgon2
	app.Now = h.Clock.Now
	app.MailerFactory = func(provider, apiKey, from string) (mail.Mailer, error) {
		return h.Mailer, nil
	}
	if h.HasZooid {
		app.Zooid = &zooid.Manager{
			ConfigDir: filepath.Join(h.zooidDir, "config"),
			Addr:      h.zooidAddr(),
		}
	}

	// A stable port across restarts: bunker URLs and sessions must keep
	// working after Restart() (BUNKER-07), as they do in production where
	// the domain never changes.
	l, err := net.Listen("tcp", "127.0.0.1:"+h.port)
	if err != nil {
		h.T.Fatal(err)
	}
	ts := &httptest.Server{Listener: l, Config: &http.Server{Handler: app.Handler()}}
	ts.Start()
	app.PublicBaseURL = ts.URL

	h.Store, h.App, h.Server = s, app, ts
	h.RelayURL = "ws" + strings.TrimPrefix(ts.URL, "http") + "/bunker"
	// As main.go does: start bunkers for all communities.
	if slugs, err := s.Slugs(); err == nil {
		for _, slug := range slugs {
			if c, err := s.Community(slug); err == nil {
				app.StartBunker(c)
			}
		}
	}
	h.T.Cleanup(func() { app.Close(); ts.Close(); s.Close() })
}

// Restart simulates a communityd restart: same data directory, fresh
// process state (keyring, caches, bunker loops). The fake mailer, clock
// and relay persist, as the outside world would.
func (h *H) Restart() {
	h.T.Helper()
	h.App.Close()
	h.Server.Close()
	h.Store.Close()
	h.boot()
}

// Community returns the single harness community.
func (h *H) Community() *store.Community {
	h.T.Helper()
	c, err := h.Store.CommunityByHost(Domain)
	if err != nil {
		h.T.Fatal(err)
	}
	return c
}

// Client returns an HTTP client that keeps cookies and does not follow
// redirects, so tests can assert on them.
func (h *H) Client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Get fetches a path with a throwaway client.
func (h *H) Get(path string) *http.Response {
	h.T.Helper()
	resp, err := h.Client().Get(h.Server.URL + path)
	if err != nil {
		h.T.Fatal(err)
	}
	return resp
}

// PostForm posts with a throwaway client.
func (h *H) PostForm(path string, v url.Values) *http.Response {
	h.T.Helper()
	resp, err := h.Client().PostForm(h.Server.URL+path, v)
	if err != nil {
		h.T.Fatal(err)
	}
	return resp
}

// CompleteSetup runs the entire wizard over real HTTP (SETUP-01..11) with
// the given strict-mode choice, returning a logged-in admin client.
func (h *H) CompleteSetup(strict bool) *http.Client {
	h.T.Helper()
	client := h.Client()
	post := func(path string, v url.Values, wantStatus int) {
		h.T.Helper()
		resp, err := client.PostForm(h.Server.URL+path, v)
		if err != nil {
			h.T.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != wantStatus {
			h.T.Fatalf("POST %s: want %d, got %d", path, wantStatus, resp.StatusCode)
		}
	}

	post("/setup", url.Values{"domain": {Domain}}, 303)
	strictVal := ""
	if strict {
		strictVal = "1"
	}
	post("/setup/password", url.Values{
		"password": {MasterPassword}, "confirm": {MasterPassword}, "strict": {strictVal},
	}, 303)
	post("/setup/admin", url.Values{"username": {"xavier"}}, 303)
	post("/setup/email", url.Values{
		"provider": {"resend"}, "api_key": {"re_test"}, "from": {"community@" + Domain},
	}, 303)
	post("/setup/verify", url.Values{"email": {AdminEmail}}, 200)
	code := h.Mailer.LastCodeTo(AdminEmail)
	post("/setup/verify", url.Values{"email": {AdminEmail}, "code": {code}}, 303)
	post("/setup/community", url.Values{
		"name": {"Commons Hub"}, "description": {"A space for commoners."},
	}, 303)
	h.Admin = client
	return client
}

// relayClient builds a publish.Client for the public /relay endpoint.
func (h *H) relayClient() (*publish.Client, *store.Community, string) {
	h.T.Helper()
	if !h.HasZooid {
		h.T.Skip("bin/zooid missing — run `make zooid` for relay coverage")
	}
	c := h.Community()
	p := &publish.Client{
		URL: "ws" + strings.TrimPrefix(h.Server.URL, "http") + "/relay",
		Signer: &bunker.Signer{
			C:   c,
			DEK: func() ([]byte, bool) { return h.App.DEK(c) },
			Now: h.Clock.Now,
		},
	}
	claim, _ := c.Setting("zooid_claim")
	return p, c, claim
}

// QueryRelayAs reads events from the data relay as an identity, through
// the full production path (proxy, NIP-42 auth, membership join).
func (h *H) QueryRelayAs(username string, filter nostr.Filter) []*nostr.Event {
	h.T.Helper()
	p, c, claim := h.relayClient()
	ident, err := c.IdentityByUsername(username)
	if err != nil {
		h.T.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	events, err := p.QueryAs(ctx, ident, claim, filter)
	if err != nil {
		h.T.Fatalf("relay query as %s: %v", username, err)
	}
	return events
}

// PublishRelayAs publishes an event as an identity over raw nostr — the
// "external client" path (CHAT-06).
func (h *H) PublishRelayAs(t *testing.T, username string, evt *nostr.Event) {
	t.Helper()
	p, c, claim := h.relayClient()
	ident, err := c.IdentityByUsername(username)
	if err != nil {
		t.Fatal(err)
	}
	if evt.CreatedAt == 0 {
		evt.CreatedAt = nostr.Timestamp(h.Clock.Now().Unix())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.PublishAs(ctx, ident, claim, evt); err != nil {
		t.Fatalf("relay publish as %s: %v", username, err)
	}
}

// Member creates an active member through the front door: a real
// application, email verification, and an admin approval — then logs them
// in. Extra roles are granted directly in the store (no roles UI yet).
func (h *H) Member(username string, roles ...string) *http.Client {
	h.T.Helper()
	if h.Admin == nil {
		h.T.Fatal("harness: CompleteSetup before Member")
	}
	email := username + "@example.org"
	client := h.Client()
	post := func(cl *http.Client, path string, v url.Values) {
		h.T.Helper()
		resp, err := cl.PostForm(h.Server.URL+path, v)
		if err != nil {
			h.T.Fatal(err)
		}
		resp.Body.Close()
	}

	post(client, "/join", url.Values{
		"name": {username}, "username": {username}, "email": {email},
		"motivation": {"harness fixture for " + username}, "newsletter": {"1"},
	})
	post(client, "/join/verify", url.Values{
		"email": {email}, "code": {h.Mailer.LastCodeTo(email)},
	})

	c := h.Community()
	app, err := c.OpenApplicationByEmail(email)
	if err != nil {
		h.T.Fatalf("harness: application for %s: %v", username, err)
	}
	post(h.Admin, fmt.Sprintf("/members/pending/%d", app.ID), url.Values{"decision": {"approve"}})

	post(client, "/login", url.Values{"email": {email}})
	post(client, "/login", url.Values{"email": {email}, "code": {h.Mailer.LastCodeTo(email)}})

	ident, err := c.IdentityByUsername(username)
	if err != nil {
		h.T.Fatal(err)
	}
	for _, role := range roles {
		if err := c.AssignRole(ident.ID, role); err != nil {
			h.T.Fatal(err)
		}
	}
	if len(roles) > 0 {
		// Role changes project into zooid's config (moderators get
		// can_manage); give its hot-reload a moment to pick it up.
		if err := h.App.ResyncZooid(c); err != nil {
			h.T.Fatal(err)
		}
		if h.HasZooid {
			time.Sleep(300 * time.Millisecond)
		}
	}
	return client
}

// Clock is the injectable time source — tests advance it, never sleep.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// FakeMailer captures every message in memory and has a scriptable Verify
// (docs/testing/environment.md).
type FakeMailer struct {
	mu       sync.Mutex
	Messages []mail.Message
	// DomainVerified controls Verify; default true.
	DomainVerified bool
	// MissingRecords returned when not verified.
	MissingRecords []mail.DNSRecord
	// SendErr, when set, fails Send.
	SendErr error
}

func NewFakeMailer() *FakeMailer {
	return &FakeMailer{DomainVerified: true}
}

func (f *FakeMailer) Send(_ context.Context, msg mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SendErr != nil {
		return f.SendErr
	}
	f.Messages = append(f.Messages, msg)
	return nil
}

func (f *FakeMailer) Verify(context.Context) (*mail.DomainStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &mail.DomainStatus{Verified: f.DomainVerified, Records: f.MissingRecords}, nil
}

// LastTo returns the most recent message to an address.
func (f *FakeMailer) LastTo(addr string) (mail.Message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.Messages) - 1; i >= 0; i-- {
		for _, to := range f.Messages[i].To {
			if to == addr {
				return f.Messages[i], true
			}
		}
	}
	return mail.Message{}, false
}

var codeRe = regexp.MustCompile(`\b([0-9]{6})\b`)

// LastCodeTo extracts the 6-digit code from the latest email to addr.
func (f *FakeMailer) LastCodeTo(addr string) string {
	msg, ok := f.LastTo(addr)
	if !ok {
		return ""
	}
	m := codeRe.FindStringSubmatch(msg.Text)
	if m == nil {
		return ""
	}
	return m[1]
}

// Count returns how many messages were sent to addr.
func (f *FakeMailer) Count(addr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.Messages {
		for _, to := range m.To {
			if to == addr {
				n++
			}
		}
	}
	return n
}
