package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// The admin activity log (docs/decisions/0011). Every signed event on the
// community's relay rendered as a human line with a raw-JSON inspector.
// The log IS the relay — there is no separate audit store, so it survives
// the app database (LOG-05) and is reconstructed on every view.

const logPageSize = 50

type logEntry struct {
	Line     string
	Category string
	Actor    string
	Time     string
	JSON     string
}

func (a *App) logPage(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	p, ok := a.publisher(c)
	if !ok {
		a.render(w, "log.html", map[string]any{"Title": "Activity log", "Entries": nil})
		return
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	// All events readable by the community identity (a relay member),
	// across every group it belongs to.
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{Limit: 1000})
	if err != nil {
		a.internalError(w, err)
		return
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt > events[j].CreatedAt })

	categoryFilter := r.URL.Query().Get("category")
	authorFilter := r.URL.Query().Get("author")
	var authorPK string
	if authorFilter != "" {
		if ident, err := c.IdentityByUsername(authorFilter); err == nil {
			authorPK = ident.Pubkey
		}
	}

	var all []logEntry
	for _, evt := range events {
		line, cat := a.describeEvent(c, community.Pubkey, evt)
		if categoryFilter != "" && cat != categoryFilter {
			continue
		}
		if authorFilter != "" && evt.PubKey != authorPK {
			continue
		}
		raw, _ := json.MarshalIndent(evt, "", "  ")
		actor := a.actorName(c, community.Pubkey, evt.PubKey)
		all = append(all, logEntry{
			Line: line, Category: cat, Actor: actor,
			Time: evt.CreatedAt.Time().UTC().Format("2006-01-02 15:04"),
			JSON: string(raw),
		})
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	start := (page - 1) * logPageSize
	end := start + logPageSize
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}

	a.render(w, "log.html", map[string]any{
		"Title": "Activity log", "Entries": all[start:end],
		"Page": page, "HasNext": end < len(all), "HasPrev": page > 1,
		"Category": categoryFilter, "Author": authorFilter,
		"Categories": []string{"publishing", "members", "channels", "profile", "money"},
	})
}

func (a *App) actorName(c *store.Community, communityPK, pubkey string) string {
	if pubkey == communityPK {
		return "the community"
	}
	if ident, err := c.IdentityByPubkey(pubkey); err == nil {
		return "@" + ident.Username
	}
	return pubkey[:8] + "…"
}

// describeEvent renders one event as a human line and assigns a category.
// Unknown kinds get a generic line, never hidden (LOG-02).
func (a *App) describeEvent(c *store.Community, communityPK string, evt *nostr.Event) (string, string) {
	actor := a.actorName(c, communityPK, evt.PubKey)
	byCommunity := evt.PubKey == communityPK
	h := tagVal(evt, "h")

	switch evt.Kind {
	case 0:
		if byCommunity {
			return "the community profile was published", "profile"
		}
		return actor + " updated their profile", "members"
	case 3:
		return actor + " followed the community", "members"
	case publish.KindAnnouncement:
		if byCommunity {
			return "the community published an announcement", "publishing"
		}
		return actor + " proposed an announcement", "publishing"
	case 5:
		return actor + " removed an event", "channels"
	case int(nostr.KindReaction):
		return actor + " reacted " + evt.Content, "channels"
	case nostr.KindSimpleGroupChatMessage:
		return actor + " posted in #general", "channels"
	case publish.KindThreadRoot:
		return actor + " started a thread in " + channelName(h), categoryForChannel(h)
	case publish.KindComment:
		switch tagVal(evt, "t") {
		case "payment-claim":
			return actor + " claimed a payment", "money"
		case "payment-confirm":
			return actor + " confirmed a payment", "money"
		}
		return actor + " replied in " + channelName(h), categoryForChannel(h)
	case publish.KindLabel:
		switch {
		case evt.Tags.GetFirst([]string{"l", "declined"}) != nil:
			return actor + " declined a proposal", "publishing"
		case evt.Tags.GetFirst([]string{"l", "paid"}) != nil:
			return actor + " marked an expense paid", "money"
		case evt.Tags.GetFirst([]string{"l", "cancelled"}) != nil:
			return actor + " cancelled an event", "channels"
		}
		return actor + " labelled an event", "channels"
	case publish.KindApproval:
		return actor + " approved a proposal", "publishing"
	case publish.KindArticle:
		kind := "a blog post"
		if publish.IsNewsletter(evt) {
			kind = "a newsletter"
		}
		if byCommunity {
			return "the community published " + kind, "publishing"
		}
		return actor + " proposed " + kind, "publishing"
	case publish.KindAppData:
		return actor + " proposed a profile change", "profile"
	case publish.KindDateEvent, publish.KindTimeEvent:
		return actor + " created an event", "channels"
	case publish.KindRSVP:
		return actor + " RSVP'd " + tagVal(evt, "status"), "channels"
	case publish.KindLedger:
		return actor + " recorded a " + tagVal(evt, "t"), "money"
	case publish.KindCommunityDefinition:
		return "the community definition was published", "publishing"
	default:
		return fmt.Sprintf("%s signed a kind %d event", actor, evt.Kind), "other"
	}
}

func channelName(h string) string {
	if h == "" {
		return "a channel"
	}
	return h
}

func categoryForChannel(h string) string {
	if h == expensesSlug {
		return "money"
	}
	return "channels"
}
