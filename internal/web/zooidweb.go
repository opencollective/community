package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
	"github.com/opencollective/community/internal/zooid"
)

// zooid integration: per-community config, the /relay + Blossom reverse
// proxy, and the event-publishing hooks (docs/architecture/overview.md).

const (
	setZooidClaim  = "zooid_claim"
	publishTimeout = 10 * time.Second
)

// publicURL builds an externally reachable URL for the community. The
// harness overrides PublicBaseURL (httptest); production derives from the
// hostname.
func (a *App) publicURL(c *store.Community, scheme, path string) string {
	if a.PublicBaseURL != "" {
		base := strings.TrimPrefix(a.PublicBaseURL, "http")
		return scheme + base + path
	}
	host := c.Hostname
	if !a.DevMode {
		return scheme + "s://" + host + path
	}
	return scheme + "://" + host + path
}

// relayURLFor is the NIP-46 transport URL (the embedded /bunker relay).
func (a *App) relayURLFor(c *store.Community) (string, bool) {
	return a.publicURL(c, "ws", "/bunker"), true
}

// dataRelayURLFor is the community data relay (zooid via our proxy).
func (a *App) dataRelayURLFor(c *store.Community) (string, bool) {
	if a.Zooid == nil {
		return "", false
	}
	return a.publicURL(c, "ws", "/relay"), true
}

// syncZooid refreshes the community's zooid config from current settings
// and roles: moderate_chat holders (and the admin) become can_manage
// pubkeys so their NIP-29 moderation events carry their own signatures.
func (a *App) syncZooid(c *store.Community) error {
	if a.Zooid == nil {
		return nil
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return err
	}
	managers, err := c.PubkeysWithPermission(store.PermModerateChat)
	if err != nil {
		return err
	}
	if _, admin, err := a.adminIdentity(c); err == nil {
		managers = append(managers, admin.Pubkey)
	}
	name, _ := c.Setting(setName)
	desc, _ := c.Setting(setDescription)
	icon, _ := c.Setting(setIcon)
	return a.Zooid.WriteConfig(c, community.Pubkey, name, desc, icon, managers)
}

// ResyncZooid is the exported hook for role changes (the harness and the
// future roles UI call it).
func (a *App) ResyncZooid(c *store.Community) error { return a.syncZooid(c) }

func (a *App) communityIdentity(c *store.Community) (*store.Identity, error) {
	idStr, err := c.Setting(setCommunityID)
	if err != nil {
		return nil, err
	}
	var id int64
	fmt.Sscan(idStr, &id)
	return c.IdentityByID(id)
}

// publisher returns a client for the community's data relay.
func (a *App) publisher(c *store.Community) (*publish.Client, bool) {
	u, ok := a.dataRelayURLFor(c)
	if !ok {
		return nil, false
	}
	return &publish.Client{URL: u, Signer: a.SignerFor(c)}, true
}

// claim returns the relay invite code, fetching and caching it on first
// use (the community identity's can_invite role authorizes the read).
func (a *App) claim(ctx context.Context, c *store.Community, p *publish.Client) (string, error) {
	if v, err := c.Setting(setZooidClaim); err == nil && v != "" {
		return v, nil
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return "", err
	}
	claim, err := p.Claim(ctx, community)
	if err != nil {
		return "", err
	}
	return claim, c.SetSetting(setZooidClaim, claim)
}

// publishIdentityEvents pushes an identity's kind 0 profile and kind 3
// follow of the community to the relay. Best-effort: relay unavailability
// never fails a user flow; it is logged and the events re-publish on the
// next occasion.
func (a *App) publishIdentityEvents(c *store.Community, ident *store.Identity) {
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	community, err := a.communityIdentity(c)
	if err != nil {
		a.Log.Error("publish identity: community identity", "err", err)
		return
	}
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.Log.Error("publish identity: claim", "err", err)
		return
	}
	err = p.PublishAs(ctx, ident, claim,
		publish.ProfileEvent(displayName(ident), "", "", a.Now()),
		publish.FollowEvent(community.Pubkey, a.Now()),
	)
	if err != nil {
		a.Log.Error("publish identity events", "username", ident.Username, "err", err)
	}
}

// publishCommunityEvents pushes the community's kind 0 profile and its
// NIP-72 definition (SETUP-11's relay half).
func (a *App) publishCommunityEvents(c *store.Community) {
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	community, err := a.communityIdentity(c)
	if err != nil {
		a.Log.Error("publish community: identity", "err", err)
		return
	}
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.Log.Error("publish community: claim", "err", err)
		return
	}
	name, _ := c.Setting(setName)
	desc, _ := c.Setting(setDescription)
	icon, _ := c.Setting(setIcon)
	err = p.PublishAs(ctx, community, claim,
		publish.ProfileEvent(name, desc, icon, a.Now()),
		publish.CommunityDefinitionEvent(c.Slug, name, desc, icon, nil, a.Now()),
	)
	if err != nil {
		a.Log.Error("publish community events", "err", err)
	}
}

// generalGroup is the slug (NIP-29 `h` tag) of the default chat channel.
const generalGroup = "general"

// createGeneralChannel publishes the #general NIP-29 group: created and
// marked private+closed by the community identity, with the admin as its
// first member (SETUP-11).
func (a *App) createGeneralChannel(c *store.Community) {
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	community, err := a.communityIdentity(c)
	if err != nil {
		a.Log.Error("create #general: community identity", "err", err)
		return
	}
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.Log.Error("create #general: claim", "err", err)
		return
	}
	_, admin, err := a.adminIdentity(c)
	if err != nil {
		a.Log.Error("create #general: admin", "err", err)
		return
	}
	err = p.PublishAs(ctx, community, claim,
		publish.GroupCreateEvent(generalGroup, a.Now()),
		publish.GroupMetadataEvent(generalGroup, "general",
			"The community's living room.", true, true, a.Now()),
		publish.GroupPutUserEvent(generalGroup, admin.Pubkey, a.Now()),
	)
	if err != nil {
		a.Log.Error("create #general", "err", err)
	}
}

// addToGeneral grants a member access to #general (JOIN-05, CHAT-07).
func (a *App) addToGeneral(c *store.Community, ident *store.Identity) {
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	community, err := a.communityIdentity(c)
	if err != nil {
		return
	}
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return
	}
	if err := p.PublishAs(ctx, community, claim,
		publish.GroupPutUserEvent(generalGroup, ident.Pubkey, a.Now())); err != nil {
		a.Log.Error("add to #general", "username", ident.Username, "err", err)
	}
}

func displayName(ident *store.Identity) string {
	if ident.Name != "" {
		return ident.Name
	}
	return ident.Username
}

// --- reverse proxy ---

// zooidProxy forwards to zooid. The Host header is rewritten to the
// community's configured virtual-relay host (zooid dispatches by exact
// match); X-Forwarded-Host/-Proto carry the original so zooid's NIP-42
// validation reconstructs exactly the URL the client dialed. Paths pass
// through — khatru serves the websocket on any path.
func (a *App) zooidProxy() http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			c := communityFrom(pr.In)
			pr.SetURL(&url.URL{Scheme: "http", Host: a.Zooid.Addr})
			pr.Out.URL.Path = pr.In.URL.Path
			pr.SetXForwarded()
			if c != nil {
				pr.Out.Host = zooid.HostFor(c)
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Zooid == nil || communityFrom(r) == nil {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

func isBlossomHash(path string) bool {
	if len(path) != 65 || path[0] != '/' {
		return false
	}
	for _, ch := range path[1:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
