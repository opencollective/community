// Package store implements the storage layout of
// docs/architecture/storage.md and the fractal tenancy of ADR 0009:
// a thin server.db registry plus one self-contained SQLite database per
// community under communities/<slug>/.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/server/*.sql
var serverMigrations embed.FS

//go:embed migrations/community/*.sql
var communityMigrations embed.FS

var ErrNotFound = errors.New("store: not found")

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Server is the per-machine registry: which communities live here, under
// which hostnames. It holds no community content (TEN-04).
type Server struct {
	DataDir string

	db *sql.DB

	mu     sync.Mutex
	opened map[string]*Community
}

// OpenServer opens (creating if needed) the registry at dataDir/server.db
// and prepares the communities/ directory.
func OpenServer(dataDir string) (*Server, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "communities"), 0o750); err != nil {
		return nil, err
	}
	db, err := openSQLite(filepath.Join(dataDir, "server.db"))
	if err != nil {
		return nil, err
	}
	if err := migrate(db, serverMigrations, "migrations/server"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: server migrations: %w", err)
	}
	return &Server{DataDir: dataDir, db: db, opened: map[string]*Community{}}, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.opened {
		c.DB.Close()
	}
	s.opened = map[string]*Community{}
	return s.db.Close()
}

// Slugs lists all community slugs on this server.
func (s *Server) Slugs() ([]string, error) {
	rows, err := s.db.Query(`SELECT slug FROM communities ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

// CommunityCount reports how many communities exist on this server.
func (s *Server) CommunityCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM communities`).Scan(&n)
	return n, err
}

// CreateCommunity registers a community and creates its self-contained
// directory (app.db, secrets/).
func (s *Server) CreateCommunity(slug, hostname string) (*Community, error) {
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("store: invalid slug %q", slug)
	}
	hostname = NormalizeHost(hostname)
	dir := filepath.Join(s.DataDir, "communities", slug)
	if err := os.MkdirAll(filepath.Join(dir, "secrets"), 0o700); err != nil {
		return nil, err
	}
	_, err := s.db.Exec(
		`INSERT INTO communities (slug, hostname, created_at) VALUES (?, ?, ?)`,
		slug, hostname, time.Now().UTC().Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("store: register community: %w", err)
	}
	return s.Community(slug)
}

// Community opens (or returns the already-open) community by slug.
func (s *Server) Community(slug string) (*Community, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.opened[slug]; ok {
		return c, nil
	}
	var hostname string
	var parent sql.NullString
	err := s.db.QueryRow(`SELECT hostname, parent FROM communities WHERE slug = ?`, slug).
		Scan(&hostname, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.DataDir, "communities", slug)
	db, err := openSQLite(filepath.Join(dir, "app.db"))
	if err != nil {
		return nil, err
	}
	if err := migrate(db, communityMigrations, "migrations/community"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: community migrations: %w", err)
	}
	c := &Community{Slug: slug, Hostname: hostname, Parent: parent.String, Dir: dir, DB: db}
	s.opened[slug] = c
	return c, nil
}

// CommunityByHost resolves the Host header to a community — the tenant
// resolution middleware's source of truth (TEN-01).
func (s *Server) CommunityByHost(host string) (*Community, error) {
	host = NormalizeHost(host)
	var slug string
	err := s.db.QueryRow(`SELECT slug FROM communities WHERE hostname = ?`, host).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Community(slug)
}

// NormalizeHost lowercases and strips any port from a Host header value.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

// Community is one self-contained tenant (ADR 0009).
type Community struct {
	Slug     string
	Hostname string
	Parent   string
	Dir      string
	DB       *sql.DB
}

// Setting reads a settings value; missing keys return ErrNotFound.
func (c *Community) Setting(key string) (string, error) {
	var v string
	err := c.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

// SetSetting writes a settings value.
func (c *Community) SetSetting(key, value string) error {
	_, err := c.DB.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	// SQLite handles one writer at a time; serialize at the pool level.
	db.SetMaxOpenConns(1)
	return db, nil
}

// migrate applies numbered .sql files not yet recorded in PRAGMA user_version.
func migrate(db *sql.DB, fsys embed.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i, name := range names {
		n := i + 1
		if n <= version {
			continue
		}
		body, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", n)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
