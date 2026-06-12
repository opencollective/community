// communityd is the Community server: web app, bunker, NIP-05, email and
// reverse proxy in one binary (docs/architecture/overview.md).
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/web"
)

var version = "dev" // set by -ldflags at release time

func main() {
	var (
		dataDir     = flag.String("data", envOr("DATA", "./data"), "data directory")
		addr        = flag.String("addr", envOr("ADDR", ":8080"), "dev listen address (plain HTTP)")
		prod        = flag.Bool("prod", os.Getenv("MODE") == "prod", "production mode: ports 80/443 with automatic certificates")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("communityd", version)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s, err := store.OpenServer(*dataDir)
	if err != nil {
		log.Error("open data dir", "dir", *dataDir, "err", err)
		os.Exit(1)
	}
	defer s.Close()

	app, err := web.New(s, log)
	if err != nil {
		log.Error("init web", "err", err)
		os.Exit(1)
	}
	defer app.Close()

	// Start NIP-46 bunkers for communities with a configured relay.
	if slugs, err := s.Slugs(); err == nil {
		for _, slug := range slugs {
			if c, err := s.Community(slug); err == nil {
				app.StartBunker(c)
			}
		}
	}

	if *prod {
		runProd(s, app, *dataDir, log)
		return
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Info("communityd listening (dev)", "addr", *addr, "data", *dataDir, "version", version)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

// runProd serves :80 (wizard step 1, ACME challenges, HTTPS redirects) and
// :443 with certificates issued on demand for registered hostnames
// (flows/setup.md steps 1 and 3 of SETUP-02/03).
func runProd(s *store.Server, app *web.App, dataDir string, log *slog.Logger) {
	app.DevMode = false

	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(filepath.Join(dataDir, "..", "acme")),
		HostPolicy: func(_ context.Context, host string) error {
			if _, err := s.CommunityByHost(host); err != nil {
				return fmt.Errorf("host %q is not served here", host)
			}
			return nil
		},
	}

	handler := app.Handler()

	// :80 — ACME HTTP-01, the pre-TLS wizard, and redirects to HTTPS.
	httpHandler := m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.CommunityByHost(r.Host); err == nil {
			target := "https://" + strings.Split(r.Host, ":")[0] + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		handler.ServeHTTP(w, r) // first run: the wizard over plain HTTP
	}))
	go func() {
		srv := &http.Server{Addr: ":80", Handler: httpHandler, ReadHeaderTimeout: 10 * time.Second}
		if err := srv.ListenAndServe(); err != nil {
			log.Error("http listener", "err", err)
			os.Exit(1)
		}
	}()

	srv := &http.Server{
		Addr:              ":443",
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: m.GetCertificate,
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"h2", "http/1.1"},
		},
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		log.Error("tls listener", "err", err)
		os.Exit(1)
	}
	log.Info("communityd listening (prod)", "http", ":80", "https", ":443", "version", version)
	if err := srv.Serve(tls.NewListener(ln, srv.TLSConfig)); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
