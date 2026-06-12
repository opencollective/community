// Package harness boots an isolated communityd for integration tests
// (docs/testing/environment.md). zooid, the fake mailer and the fake clock
// join it in their respective milestones; the shape is in place now so
// tests written today don't churn later.
package harness

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/web"
)

// H is one isolated server instance backed by a temp directory.
type H struct {
	T      *testing.T
	Store  *store.Server
	Server *httptest.Server
}

// New boots a fresh server with an empty data directory.
func New(t *testing.T) *H {
	t.Helper()
	s, err := store.OpenServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	app, err := web.New(s, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return &H{T: t, Store: s, Server: ts}
}

// Client returns an HTTP client that does not follow redirects, so tests
// can assert on them.
func (h *H) Client() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Get fetches a path and fails the test on transport errors.
func (h *H) Get(path string) *http.Response {
	h.T.Helper()
	resp, err := h.Client().Get(h.Server.URL + path)
	if err != nil {
		h.T.Fatal(err)
	}
	return resp
}
