package web

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// Thread channels (docs/nostr/channels.md): every thread is pending →
// approved per the channel's policy; visibility is per thread; all state
// lives on the relay as signed events.

// templates' required fields, per docs/nostr/channels.md.
var threadTemplates = map[string]struct {
	NeedsTitle bool
}{
	"proposal": {NeedsTitle: true},
	"request":  {NeedsTitle: false},
}

type thread struct {
	ID         string
	Title      string
	Content    string
	Author     string
	AuthorPK   string
	Visibility string
	Status     string // pending | approved | declined
	Time       string
	Replies    []reply
	Reactions  []reaction
	Approvers  []string // usernames whose approvals currently count
	Required   int
	DeclinedBy string
	Reason     string
}

type reply struct {
	ID      string
	Author  string
	Content string
	Time    string
}

type reaction struct {
	Emoji string
	Count int
}

// channelThreads reads and assembles a channel's full thread state from
// the relay (status recomputed live — approvals re-checked against
// current roles, PUB-10 semantics).
func (a *App) channelThreads(c *store.Community, ch *store.Channel) ([]*thread, error) {
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
		Kinds: []int{publish.KindThreadRoot, publish.KindComment,
			nostr.KindReaction, nostr.KindDeletion,
			publish.KindApproval, publish.KindLabel},
		Tags:  nostr.TagMap{"h": []string{ch.Slug}},
		Limit: 500,
	})
	if err != nil {
		return nil, err
	}

	byKind := map[int][]*nostr.Event{}
	for _, evt := range events {
		byKind[evt.Kind] = append(byKind[evt.Kind], evt)
	}

	// Reactions retracted by their own author (kind 5) don't count.
	retracted := map[string]bool{} // author + ":" + target event id
	for _, del := range byKind[int(nostr.KindDeletion)] {
		for _, tag := range del.Tags.GetAll([]string{"e", ""}) {
			if len(tag) > 1 {
				retracted[del.PubKey+":"+tag[1]] = true
			}
		}
	}

	threads := map[string]*thread{}
	var order []string
	for _, root := range byKind[publish.KindThreadRoot] {
		t := &thread{
			ID:         root.ID,
			Content:    root.Content,
			AuthorPK:   root.PubKey,
			Visibility: ch.DefaultVisibility,
			Status:     "pending",
			Time:       formatTime(root.CreatedAt),
			Required:   ch.ApprovalsRequired,
		}
		if tag := root.Tags.GetFirst([]string{"title", ""}); tag != nil && len(*tag) > 1 {
			t.Title = (*tag)[1]
		}
		if tag := root.Tags.GetFirst([]string{"visibility", ""}); tag != nil && len(*tag) > 1 {
			t.Visibility = (*tag)[1]
		}
		if ident, err := c.IdentityByPubkey(root.PubKey); err == nil {
			t.Author = ident.Username
		} else {
			t.Author = root.PubKey[:8] + "…"
		}
		threads[root.ID] = t
		order = append(order, root.ID)
	}

	for _, evt := range byKind[publish.KindComment] {
		if rootID := firstETag(evt); rootID != "" {
			if t := threads[rootID]; t != nil {
				r := reply{ID: evt.ID, Content: evt.Content, Time: formatTime(evt.CreatedAt)}
				if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
					r.Author = ident.Username
				} else {
					r.Author = evt.PubKey[:8] + "…"
				}
				t.Replies = append(t.Replies, r)
			}
		}
	}

	// One active reaction per identity × emoji × target (CHAN-06).
	counts := map[string]map[string]map[string]bool{} // root -> emoji -> author
	for _, evt := range byKind[int(nostr.KindReaction)] {
		target := firstETag(evt)
		if target == "" || retracted[evt.PubKey+":"+evt.ID] == true {
			continue
		}
		t := threads[target]
		if t == nil {
			continue
		}
		if counts[target] == nil {
			counts[target] = map[string]map[string]bool{}
		}
		if counts[target][evt.Content] == nil {
			counts[target][evt.Content] = map[string]bool{}
		}
		counts[target][evt.Content][evt.PubKey] = true
	}
	for rootID, emojis := range counts {
		t := threads[rootID]
		var keys []string
		for e := range emojis {
			keys = append(keys, e)
		}
		sort.Strings(keys)
		for _, e := range keys {
			t.Reactions = append(t.Reactions, reaction{Emoji: e, Count: len(emojis[e])})
		}
	}

	// Approvals: distinct approvers, author excluded, role re-checked now
	// (CHAN-15/16); the admin alone decides.
	for _, evt := range byKind[publish.KindApproval] {
		rootID := firstETag(evt)
		t := threads[rootID]
		if t == nil || evt.PubKey == t.AuthorPK {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, ch.ApproveRoles)
		if !isAdmin && !holds {
			continue
		}
		dup := false
		for _, u := range t.Approvers {
			if u == ident.Username {
				dup = true
			}
		}
		if !dup {
			t.Approvers = append(t.Approvers, ident.Username)
		}
		if isAdmin || len(t.Approvers) >= ch.ApprovalsRequired {
			t.Status = "approved"
		}
	}

	for _, evt := range byKind[publish.KindLabel] {
		if evt.Tags.GetFirst([]string{"l", "declined"}) == nil {
			continue
		}
		rootID := firstETag(evt)
		t := threads[rootID]
		if t == nil || t.Status == "approved" {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, ch.ApproveRoles)
		if !isAdmin && !holds {
			continue
		}
		t.Status = "declined"
		t.DeclinedBy = ident.Username
		t.Reason = evt.Content
	}

	out := make([]*thread, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- { // newest first
		out = append(out, threads[order[i]])
	}
	return out, nil
}

func firstETag(evt *nostr.Event) string {
	if tag := evt.Tags.GetFirst([]string{"e", ""}); tag != nil && len(*tag) > 1 {
		return (*tag)[1]
	}
	return ""
}

func formatTime(ts nostr.Timestamp) string {
	return ts.Time().UTC().Format("Jan 2 15:04")
}

// channelAccess resolves a channel and what the viewer may see: members
// see everything; everyone else sees approved + public threads only
// (CHAN-20, ADR 0012).
func (a *App) channelAccess(r *http.Request) (*store.Community, *store.Channel, *store.Identity, bool, error) {
	c := communityFrom(r)
	if c == nil {
		return nil, nil, nil, false, store.ErrNotFound
	}
	ch, err := c.ChannelBySlug(r.PathValue("slug"))
	if err != nil || !ch.Enabled || ch.Type != "threads" {
		return nil, nil, nil, false, store.ErrNotFound
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	return c, ch, viewer, isMember, nil
}

func (a *App) channelList(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, isMember, err := a.channelAccess(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	threads, err := a.channelThreads(c, ch)
	if err != nil {
		a.internalError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	var visible []*thread
	for _, t := range threads {
		if !isMember && (t.Status != "approved" || t.Visibility != "public") {
			continue
		}
		if isMember && (status == "pending" || status == "approved") && t.Status != status {
			continue
		}
		if t.Status == "declined" && status != "declined" {
			continue
		}
		visible = append(visible, t)
	}
	canDecide := false
	if viewer != nil {
		canDecide = a.isAdmin(c, viewer.ID)
		if !canDecide {
			canDecide, _ = c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		}
	}
	a.render(w, "channel_list.html", map[string]any{
		"Title": ch.Name, "Channel": ch, "Threads": visible,
		"IsMember": isMember, "Status": status, "CanDecide": canDecide,
	})
}

func (a *App) channelNewForm(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, isMember, err := a.channelAccess(r)
	_ = c
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if viewer == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !isMember {
		http.NotFound(w, r) // externals arrive with the requests milestone
		return
	}
	a.renderChannelNew(w, ch, "", "", "")
}

func (a *App) renderChannelNew(w http.ResponseWriter, ch *store.Channel, title, content, errMsg string) {
	tmpl := threadTemplates[ch.Template]
	a.render(w, "channel_new.html", map[string]any{
		"Title": "New in " + ch.Name, "Channel": ch,
		"NeedsTitle": tmpl.NeedsTitle, "FormTitle": title,
		"Content": content, "Error": errMsg,
	})
}

func (a *App) channelCreate(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, isMember, err := a.channelAccess(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if viewer == nil || !isMember {
		http.NotFound(w, r)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	visibility := r.FormValue("visibility")

	tmpl := threadTemplates[ch.Template]
	// Server-side template validation (CHAN-13): nothing reaches the
	// bunker or the relay on violation.
	switch {
	case tmpl.NeedsTitle && title == "":
		a.renderChannelNew(w, ch, title, content, "A title is required.")
		return
	case content == "" || len(content) > 10000:
		a.renderChannelNew(w, ch, title, content, "Write something (under 10k characters).")
		return
	}
	// Visibility: the channel decides the default and whether the author
	// may override (CHAN-19, ADR 0012).
	switch {
	case visibility == "":
		visibility = ch.DefaultVisibility
	case visibility != "public" && visibility != "members":
		a.renderChannelNew(w, ch, title, content, "Unknown visibility.")
		return
	case !ch.Overridable && visibility != ch.DefaultVisibility:
		a.renderChannelNew(w, ch, title, content, "This channel's visibility is fixed.")
		return
	}

	p, ok := a.publisher(c)
	if !ok {
		a.renderChannelNew(w, ch, title, content, "The relay is not available.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	evt := publish.ThreadRootEvent(ch.Slug, title, content, visibility, nil, a.Now())
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("thread create", "channel", ch.Slug, "err", err)
		a.renderChannelNew(w, ch, title, content, "Could not publish — try again.")
		return
	}
	http.Redirect(w, r, "/channels/"+ch.Slug, http.StatusSeeOther)
}

// threadAndChannel finds one thread, enforcing visibility for outsiders.
func (a *App) threadAndChannel(w http.ResponseWriter, r *http.Request) (*store.Community, *store.Channel, *store.Identity, *thread, bool) {
	c, ch, viewer, isMember, err := a.channelAccess(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, nil, false
	}
	threads, err := a.channelThreads(c, ch)
	if err != nil {
		a.internalError(w, err)
		return nil, nil, nil, nil, false
	}
	id := r.PathValue("id")
	for _, t := range threads {
		if t.ID != id {
			continue
		}
		if !isMember && (t.Status != "approved" || t.Visibility != "public") {
			break // outsiders get the same neutral not-found
		}
		return c, ch, viewer, t, isMember
	}
	http.NotFound(w, r)
	return nil, nil, nil, nil, false
}

func (a *App) threadView(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, t, isMember := a.threadAndChannel(w, r)
	if t == nil {
		return
	}
	canDecide := false
	if viewer != nil {
		canDecide = a.isAdmin(c, viewer.ID)
		if !canDecide {
			canDecide, _ = c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		}
	}
	a.render(w, "channel_thread.html", map[string]any{
		"Title": threadTitle(t), "Channel": ch, "T": t,
		"IsMember": isMember, "CanDecide": canDecide,
	})
}

func threadTitle(t *thread) string {
	if t.Title != "" {
		return t.Title
	}
	if len(t.Content) > 40 {
		return t.Content[:40] + "…"
	}
	return t.Content
}

// threadAct handles member interactions: reply, react, approve, decline.
func (a *App) threadAct(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, t, isMember := a.threadAndChannel(w, r)
	if t == nil {
		return
	}
	if viewer == nil || !isMember {
		http.NotFound(w, r)
		return
	}
	action := r.PathValue("action")

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

	var evt *nostr.Event
	switch action {
	case "reply":
		content := strings.TrimSpace(r.FormValue("content"))
		if content == "" || len(content) > 5000 {
			http.Redirect(w, r, threadPath(ch, t), http.StatusSeeOther)
			return
		}
		evt = publish.ReplyEvent(ch.Slug, t.ID, content, a.Now())

	case "react":
		emoji := strings.TrimSpace(r.FormValue("emoji"))
		if emoji == "" || len(emoji) > 8 {
			http.Redirect(w, r, threadPath(ch, t), http.StatusSeeOther)
			return
		}
		// Toggle: an existing identical reaction is retracted instead
		// (CHAN-06).
		if existing := a.findOwnReaction(c, ch, viewer, t.ID, emoji); existing != "" {
			evt = publish.DeletionEvent(ch.Slug, existing, a.Now())
		} else {
			evt = publish.ReactionEvent(ch.Slug, t.ID, emoji, a.Now())
		}

	case "approve", "decline":
		isAdmin := a.isAdmin(c, viewer.ID)
		holds, _ := c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		if !isAdmin && !holds {
			http.NotFound(w, r)
			return
		}
		if viewer.Pubkey == t.AuthorPK {
			http.Redirect(w, r, threadPath(ch, t)+"?err=own", http.StatusSeeOther)
			return
		}
		if action == "approve" {
			root := a.fetchRoot(c, ch, t.ID)
			if root == nil {
				http.NotFound(w, r)
				return
			}
			community, err := a.communityIdentity(c)
			if err != nil {
				a.internalError(w, err)
				return
			}
			evt = publish.ApprovalEvent(ch.Slug, root, community.Pubkey, c.Slug, a.Now())
		} else {
			evt = publish.DeclineEvent(ch.Slug, t.ID, strings.TrimSpace(r.FormValue("reason")), a.Now())
		}

	default:
		http.NotFound(w, r)
		return
	}

	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("thread action", "action", action, "err", err)
	}
	http.Redirect(w, r, threadPath(ch, t), http.StatusSeeOther)
}

func threadPath(ch *store.Channel, t *thread) string {
	return "/channels/" + ch.Slug + "/t/" + t.ID
}

// findOwnReaction returns the id of the viewer's live reaction with this
// emoji on the target, if any.
func (a *App) findOwnReaction(c *store.Community, ch *store.Channel, viewer *store.Identity, targetID, emoji string) string {
	p, ok := a.publisher(c)
	if !ok {
		return ""
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return ""
	}
	events, err := p.QueryAs(ctx, viewer, claim, nostr.Filter{
		Kinds:   []int{int(nostr.KindReaction), int(nostr.KindDeletion)},
		Authors: []string{viewer.Pubkey},
		Tags:    nostr.TagMap{"h": []string{ch.Slug}},
	})
	if err != nil {
		return ""
	}
	deleted := map[string]bool{}
	for _, evt := range events {
		if evt.Kind == nostr.KindDeletion {
			for _, tag := range evt.Tags.GetAll([]string{"e", ""}) {
				if len(tag) > 1 {
					deleted[tag[1]] = true
				}
			}
		}
	}
	for _, evt := range events {
		if evt.Kind == nostr.KindReaction && evt.Content == emoji &&
			firstETag(evt) == targetID && !deleted[evt.ID] {
			return evt.ID
		}
	}
	return ""
}

// fetchRoot retrieves the raw root event (the approval embeds its JSON).
func (a *App) fetchRoot(c *store.Community, ch *store.Channel, id string) *nostr.Event {
	p, ok := a.publisher(c)
	if !ok {
		return nil
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return nil
	}
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{IDs: []string{id}})
	if err != nil || len(events) == 0 {
		return nil
	}
	return events[0]
}
