# 0001 — Go single binary with server-rendered HTML and htmx

Status: accepted (2026-06-10)

## Context

The product is a self-hosted server that must be trivial to install, inspect
and maintain for years. The predecessor,
[openbunker](https://github.com/opencollective/openbunker), is Next.js +
Supabase + PostgreSQL + Prisma across multiple containers — capable, but
heavy to operate and tightly coupled to managed services. Candidate stacks
considered: Go + server-rendered templates + htmx; Go API + Vite/React SPA;
Next.js.

## Decision

One Go binary (`communityd`). Views are `html/template` files embedded via
`go:embed`; interactivity (live filters, code inputs, chat) uses htmx and
small vanilla JS; styling is one hand-written CSS file. No Node toolchain
exists in the project. The excellent Go Nostr ecosystem (go-nostr, khatru —
the same family zooid is built on) covers the protocol layer.

## Consequences

- Install is "download a binary"; tests are `go test` with `httptest`; the
  only build dependency is Go (+ CGO for SQLite).
- Every page is fast and works without JS except chat and clipboard niceties.
- We give up the rich-client ecosystem; heavy client-side features (e.g. an
  in-browser Nostr client) would need a separate effort. Accepted: the bunker
  architecture intentionally keeps signing server-side, so client-side crypto
  isn't needed.
- Contributors must know Go; frontend contributions are plain HTML/CSS,
  which lowers the bar in its own way.
