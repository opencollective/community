package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/opencollective/community/internal/store"
)

// NIP-05: /.well-known/nostr.json — username@domain resolution for every
// active identity, plus _ for the community itself
// (docs/nostr/identities.md).
func (a *App) nip05(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	name := r.URL.Query().Get("name")

	names := map[string]string{}
	relays := map[string][]string{}
	dataRelay, hasRelay := a.dataRelayURLFor(c)

	add := func(username, pubkey string) {
		names[username] = pubkey
		if hasRelay {
			relays[pubkey] = []string{dataRelay}
		}
	}

	if community, err := a.communityIdentity(c); err == nil {
		if name == "" || name == "_" || name == community.Username {
			add("_", community.Pubkey)
			add(community.Username, community.Pubkey)
		}
	}

	if name != "" && name != "_" {
		ident, err := c.IdentityByUsername(name)
		if err == nil && ident.Status == "active" {
			add(ident.Username, ident.Pubkey)
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			a.internalError(w, err)
			return
		}
	} else if name == "" {
		rows, err := c.DB.Query(
			`SELECT username, pubkey FROM identities WHERE status = 'active' AND pubkey IS NOT NULL`)
		if err != nil {
			a.internalError(w, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var u, pk string
			if err := rows.Scan(&u, &pk); err != nil {
				a.internalError(w, err)
				return
			}
			add(u, pk)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(map[string]any{"names": names, "relays": relays})
}
