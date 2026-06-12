// communityd is the Community server: web app, bunker, NIP-05, email and
// reverse proxy in one binary (docs/architecture/overview.md).
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/web"
)

var version = "dev" // set by -ldflags at release time

func main() {
	var (
		dataDir     = flag.String("data", envOr("DATA", "./data"), "data directory")
		addr        = flag.String("addr", envOr("ADDR", ":8080"), "listen address (dev/plain HTTP; TLS milestone adds 80/443 + autocert)")
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

	srv := &http.Server{
		Addr:              *addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Info("communityd listening", "addr", *addr, "data", *dataDir, "version", version)
	if err := srv.ListenAndServe(); err != nil {
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
