package web

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// #general chat (docs/nostr/chat.md): messages are kind 9 group events
// signed with each member's own key; the relay is the storage (CHAT-03).

type chatMessage struct {
	ID       string
	Username string
	Name     string
	Content  string
	Time     string
}

// chatMessages reads recent #general history through the community
// identity's relay session.
func (a *App) chatMessages(c *store.Community) ([]chatMessage, error) {
	p, ok := a.publisher(c)
	if !ok {
		return nil, nil
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return nil, err
	}
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{
		Kinds: []int{nostr.KindSimpleGroupChatMessage, nostr.KindSimpleGroupDeleteEvent},
		Tags:  nostr.TagMap{"h": []string{generalGroup}},
		Limit: 100,
	})
	if err != nil {
		return nil, err
	}
	// NIP-29 moderation is group state that clients apply: kind 9005
	// events (signed by moderators) mark messages as removed (CHAT-04).
	removed := map[string]bool{}
	for _, evt := range events {
		if evt.Kind == nostr.KindSimpleGroupDeleteEvent {
			for _, tag := range evt.Tags.GetAll([]string{"e", ""}) {
				if len(tag) > 1 {
					removed[tag[1]] = true
				}
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt < events[j].CreatedAt
	})
	msgs := make([]chatMessage, 0, len(events))
	for _, evt := range events {
		if evt.Kind != nostr.KindSimpleGroupChatMessage || removed[evt.ID] {
			continue
		}
		m := chatMessage{
			ID:      evt.ID,
			Content: evt.Content,
			Time:    time.Unix(int64(evt.CreatedAt), 0).UTC().Format("15:04"),
		}
		if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
			m.Username = ident.Username
			m.Name = displayName(ident)
		} else {
			m.Username = evt.PubKey[:8] + "…"
			m.Name = m.Username
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// chatFragment renders the channel for htmx refreshes.
func (a *App) chatFragment(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	a.renderChat(w, c, viewer, "")
}

func (a *App) renderChat(w http.ResponseWriter, c *store.Community, viewer *store.Identity, errMsg string) {
	msgs, err := a.chatMessages(c)
	if err != nil {
		a.Log.Error("chat history", "err", err)
	}
	muted, _ := c.Muted(viewer.ID)
	canModerate := a.isAdmin(c, viewer.ID)
	if !canModerate {
		canModerate, _ = c.HasPermission(viewer.ID, store.PermModerateChat)
	}
	a.render(w, "chat.html", map[string]any{
		"Title": "general", "Messages": msgs,
		"Muted": muted, "CanModerate": canModerate, "Error": errMsg,
	})
}

// chatPost publishes a member's message, signed with their key (CHAT-01).
// Muted members are stopped before the bunker signs (CHAT-05).
func (a *App) chatPost(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" || len(content) > 2000 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if muted, _ := c.Muted(viewer.ID); muted {
		a.renderChat(w, c, viewer, "You are muted in this channel.")
		return
	}
	p, ok := a.publisher(c)
	if !ok {
		a.renderChat(w, c, viewer, "The relay is not available.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := p.PublishAs(ctx, viewer, claim,
		publish.ChatMessageEvent(generalGroup, content, a.Now())); err != nil {
		a.Log.Error("chat post", "username", viewer.Username, "err", err)
		a.renderChat(w, c, viewer, "Could not send — try again.")
		return
	}
	a.renderChat(w, c, viewer, "")
}

// chatDelete removes a message: a kind 9005 signed by the moderator's own
// key — their can_manage pubkey is synced into zooid's config (CHAT-04).
func (a *App) chatDelete(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	if ok := a.isAdmin(c, viewer.ID); !ok {
		if ok, _ = c.HasPermission(viewer.ID, store.PermModerateChat); !ok {
			http.NotFound(w, r)
			return
		}
	}
	eventID := r.PathValue("id")
	if len(eventID) != 64 {
		http.NotFound(w, r)
		return
	}
	p, ok := a.publisher(c)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := p.PublishAs(ctx, viewer, claim,
		publish.GroupDeleteEventEvent(generalGroup, eventID, a.Now())); err != nil {
		a.Log.Error("chat delete", "moderator", viewer.Username, "err", err)
	}
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}

// chatMute toggles a member's mute (CHAT-05).
func (a *App) chatMute(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	if ok := a.isAdmin(c, viewer.ID); !ok {
		if ok, _ = c.HasPermission(viewer.ID, store.PermModerateChat); !ok {
			http.NotFound(w, r)
			return
		}
	}
	target, err := c.IdentityByUsername(r.PathValue("username"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := c.SetMuted(target.ID, r.FormValue("muted") != "0"); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}
