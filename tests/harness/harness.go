// Package harness boots an isolated communityd for integration tests
// (docs/testing/environment.md): real HTTP stack and SQLite, fake mailer,
// fake clock, fast argon2. Fixtures go through the front door.
package harness

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/web"
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
}

// New boots a fresh server with an empty data directory.
func New(t *testing.T) *H {
	t.Helper()
	h := &H{
		T:       t,
		DataDir: t.TempDir(),
		Mailer:  NewFakeMailer(),
		Clock:   &Clock{t: time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)},
	}
	h.boot()
	return h
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
	ts := httptest.NewServer(app.Handler())

	h.Store, h.App, h.Server = s, app, ts
	h.T.Cleanup(func() { ts.Close(); s.Close() })
}

// Restart simulates a process restart: same data directory, fresh process
// state (keyring, caches). The fake mailer and clock persist, as the
// outside world would.
func (h *H) Restart() {
	h.T.Helper()
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
