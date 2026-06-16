package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/calendar"
	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// The Events channel (docs/nostr/channels.md § events): a thread channel
// whose roots are NIP-52 calendar events, with RSVPs, cancellation, two
// ICS feeds and a homepage section. It reuses the thread framework's
// approval, reply, reaction and visibility machinery.

const eventsSlug = "events"

type eventThread struct {
	ID         string
	Title      string
	Content    string
	Author     string
	AuthorPK   string
	Visibility string
	Status     string // pending | approved
	Cancelled  bool
	AllDay     bool
	Start      int64
	End        int64
	Location   string
	External   string
	Image      string
	RRule      string
	When       string
	Required   int
	Approvers  []string
	Replies    []reply
	Reactions  []reaction
	Going      []string
	Counts     map[string]int // accepted/tentative/declined
	raw        *nostr.Event
}

// eventsChannel returns the Events channel if it is enabled.
func (a *App) eventsChannel(c *store.Community) (*store.Channel, bool) {
	ch, err := c.ChannelBySlug(eventsSlug)
	if err != nil || !ch.Enabled || ch.Template != "event" {
		return nil, false
	}
	return ch, true
}

func (a *App) loadEvents(c *store.Community) ([]*eventThread, error) {
	ch, ok := a.eventsChannel(c)
	if !ok {
		return nil, nil
	}
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
		Kinds: []int{publish.KindDateEvent, publish.KindTimeEvent,
			publish.KindComment, int(nostr.KindReaction), int(nostr.KindDeletion),
			publish.KindApproval, publish.KindLabel, publish.KindRSVP},
		Tags:  nostr.TagMap{"h": []string{eventsSlug}},
		Limit: 1000,
	})
	if err != nil {
		return nil, err
	}

	byKind := map[int][]*nostr.Event{}
	for _, evt := range events {
		byKind[evt.Kind] = append(byKind[evt.Kind], evt)
	}

	threads := map[string]*eventThread{}
	var order []string
	for _, kind := range []int{publish.KindDateEvent, publish.KindTimeEvent} {
		for _, root := range byKind[kind] {
			et := &eventThread{
				ID: root.ID, Content: root.Content, AuthorPK: root.PubKey,
				Visibility: ch.DefaultVisibility, Status: "pending",
				AllDay: kind == publish.KindDateEvent, Required: ch.ApprovalsRequired,
				Counts: map[string]int{}, raw: root,
			}
			et.Title = tagVal(root, "title")
			if v := tagVal(root, "visibility"); v != "" {
				et.Visibility = v
			}
			et.Location = tagVal(root, "location")
			et.External = tagVal(root, "r")
			et.Image = tagVal(root, "image")
			et.RRule = tagVal(root, "rrule")
			et.Start = parseStart(tagVal(root, "start"), et.AllDay)
			et.End = parseStart(tagVal(root, "end"), et.AllDay)
			et.When = eventWhen(et)
			if ident, err := c.IdentityByPubkey(root.PubKey); err == nil {
				et.Author = ident.Username
			} else {
				et.Author = root.PubKey[:8] + "…"
			}
			threads[root.ID] = et
			order = append(order, root.ID)
		}
	}

	// Replies.
	for _, evt := range byKind[publish.KindComment] {
		if et := threads[firstETag(evt)]; et != nil {
			r := reply{ID: evt.ID, Content: evt.Content, Time: formatTime(evt.CreatedAt)}
			if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
				r.Author = ident.Username
			}
			et.Replies = append(et.Replies, r)
		}
	}
	// Reactions (toggle-aware).
	retracted := map[string]bool{}
	for _, del := range byKind[int(nostr.KindDeletion)] {
		for _, tag := range del.Tags.GetAll([]string{"e", ""}) {
			if len(tag) > 1 {
				retracted[del.PubKey+":"+tag[1]] = true
			}
		}
	}
	rc := map[string]map[string]map[string]bool{}
	for _, evt := range byKind[int(nostr.KindReaction)] {
		target := firstETag(evt)
		if threads[target] == nil || retracted[evt.PubKey+":"+evt.ID] {
			continue
		}
		if rc[target] == nil {
			rc[target] = map[string]map[string]bool{}
		}
		if rc[target][evt.Content] == nil {
			rc[target][evt.Content] = map[string]bool{}
		}
		rc[target][evt.Content][evt.PubKey] = true
	}
	for id, emojis := range rc {
		var keys []string
		for e := range emojis {
			keys = append(keys, e)
		}
		sort.Strings(keys)
		for _, e := range keys {
			threads[id].Reactions = append(threads[id].Reactions, reaction{Emoji: e, Count: len(emojis[e])})
		}
	}
	// RSVPs: addressable, so each member's latest is the only one stored.
	for _, evt := range byKind[publish.KindRSVP] {
		et := threads[firstETag(evt)]
		if et == nil {
			continue
		}
		status := tagVal(evt, "status")
		et.Counts[status]++
		if status == "accepted" {
			if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
				et.Going = append(et.Going, ident.Username)
			}
		}
	}
	// Approvals and cancellation.
	for _, evt := range byKind[publish.KindApproval] {
		et := threads[firstETag(evt)]
		if et == nil || evt.PubKey == et.AuthorPK {
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
		if !contains(et.Approvers, ident.Username) {
			et.Approvers = append(et.Approvers, ident.Username)
		}
		if isAdmin || len(et.Approvers) >= ch.ApprovalsRequired {
			et.Status = "approved"
		}
	}
	for _, evt := range byKind[publish.KindLabel] {
		if evt.Tags.GetFirst([]string{"l", "cancelled"}) == nil {
			continue
		}
		et := threads[firstETag(evt)]
		if et == nil {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, ch.ApproveRoles)
		if evt.PubKey == et.AuthorPK || isAdmin || holds {
			et.Cancelled = true
		}
	}

	out := make([]*eventThread, 0, len(order))
	for _, id := range order {
		out = append(out, threads[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func tagVal(evt *nostr.Event, key string) string {
	if tag := evt.Tags.GetFirst([]string{key, ""}); tag != nil && len(*tag) > 1 {
		return (*tag)[1]
	}
	return ""
}

func parseStart(s string, allDay bool) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if allDay {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func eventWhen(et *eventThread) string {
	if et.AllDay {
		return time.Unix(et.Start, 0).UTC().Format("Mon Jan 2") + " (all day)"
	}
	s := time.Unix(et.Start, 0).UTC().Format("Mon Jan 2 15:04")
	if et.RRule != "" {
		s += " · repeats"
	}
	return s
}

func (a *App) eventVisible(et *eventThread, isMember bool) bool {
	if isMember {
		return true
	}
	return et.Status == "approved" && et.Visibility == "public"
}

// --- handlers ---

func (a *App) eventList(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	ch, ok := a.eventsChannel(c)
	if !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	events, err := a.loadEvents(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	var visible []*eventThread
	for _, et := range events {
		if !a.eventVisible(et, isMember) {
			continue
		}
		if isMember && (status == "pending" || status == "approved") && et.Status != status {
			continue
		}
		visible = append(visible, et)
	}
	canDecide := false
	var token string
	if viewer != nil {
		canDecide = a.isAdmin(c, viewer.ID)
		if !canDecide {
			canDecide, _ = c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		}
	}
	if isMember {
		token, _ = c.FeedToken(viewer.ID, a.Now())
	}
	a.render(w, "events_list.html", map[string]any{
		"Title": ch.Name, "Channel": ch, "Events": visible,
		"IsMember": isMember, "CanDecide": canDecide, "Status": status,
		"MembersFeed": a.publicURL(c, "http", "/channels/events/members.ics?token="+token),
		"PublicFeed":  a.publicURL(c, "http", "/channels/events/public.ics"),
	})
}

func (a *App) eventNewForm(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	ch, ok := a.eventsChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	if viewer == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if !a.memberLevel(c, viewer) {
		http.NotFound(w, r)
		return
	}
	a.render(w, "event_new.html", map[string]any{"Title": "New event", "Channel": ch, "Error": ""})
}

func (a *App) eventCreate(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	ch, ok := a.eventsChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return
	}
	viewer := identityFrom(r)
	if viewer == nil || !a.memberLevel(c, viewer) {
		http.NotFound(w, r)
		return
	}
	fail := func(msg string) {
		a.render(w, "event_new.html", map[string]any{"Title": "New event", "Channel": ch, "Error": msg})
	}

	f := publish.EventFields{
		Title:    strings.TrimSpace(r.FormValue("title")),
		Content:  strings.TrimSpace(r.FormValue("content")),
		AllDay:   r.FormValue("all_day") == "1",
		Location: strings.TrimSpace(r.FormValue("location")),
		External: strings.TrimSpace(r.FormValue("external")),
		Image:    strings.TrimSpace(r.FormValue("image")),
		D:        randomD(),
	}
	if f.Title == "" {
		fail("A title is required.")
		return
	}
	start, ok1 := parseEventTime(r.FormValue("start"))
	end, ok2 := parseEventTime(r.FormValue("end"))
	if !ok1 || !ok2 {
		fail("Start and end are required.")
		return
	}
	if end <= start {
		fail("The event must end after it starts.")
		return
	}
	f.Start, f.End = start, end
	rrule, err := calendar.PresetToRRule(r.FormValue("repeats"), time.Unix(start, 0))
	if err != nil {
		fail("That recurrence is not valid.")
		return
	}
	f.RRule = rrule
	if f.External != "" && !okURL(f.External) {
		fail("The external link must be an http(s) URL.")
		return
	}
	if f.Image != "" && !okURL(f.Image) {
		fail("The cover image must be an http(s) URL.")
		return
	}
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = ch.DefaultVisibility
	} else if visibility != "public" && visibility != "members" {
		fail("Unknown visibility.")
		return
	} else if !ch.Overridable && visibility != ch.DefaultVisibility {
		fail("This channel's visibility is fixed.")
		return
	}

	p, _ := a.publisher(c)
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	evt := publish.EventRootEvent(eventsSlug, visibility, f, a.Now())
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("event create", "err", err)
		fail("Could not publish — try again.")
		return
	}
	http.Redirect(w, r, "/channels/events", http.StatusSeeOther)
}

func (a *App) findEvent(w http.ResponseWriter, r *http.Request) (*store.Community, *store.Channel, *store.Identity, *eventThread, bool) {
	c := communityFrom(r)
	ch, ok := a.eventsChannel(c)
	if c == nil || !ok {
		http.NotFound(w, r)
		return nil, nil, nil, nil, false
	}
	viewer := identityFrom(r)
	isMember := viewer != nil && a.memberLevel(c, viewer)
	events, err := a.loadEvents(c)
	if err != nil {
		a.internalError(w, err)
		return nil, nil, nil, nil, false
	}
	id := r.PathValue("id")
	for _, et := range events {
		if et.ID == id {
			if !a.eventVisible(et, isMember) {
				break
			}
			return c, ch, viewer, et, isMember
		}
	}
	http.NotFound(w, r)
	return nil, nil, nil, nil, false
}

func (a *App) eventView(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, et, isMember := a.findEvent(w, r)
	if et == nil {
		return
	}
	canDecide := false
	if viewer != nil {
		canDecide = a.isAdmin(c, viewer.ID)
		if !canDecide {
			canDecide, _ = c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		}
	}
	a.render(w, "event_thread.html", map[string]any{
		"Title": et.Title, "Channel": ch, "E": et,
		"IsMember": isMember, "CanDecide": canDecide,
	})
}

func (a *App) eventAct(w http.ResponseWriter, r *http.Request) {
	c, ch, viewer, et, isMember := a.findEvent(w, r)
	if et == nil {
		return
	}
	if viewer == nil || !isMember {
		http.NotFound(w, r)
		return
	}
	action := r.PathValue("action")
	p, _ := a.publisher(c)
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	path := "/channels/events/t/" + et.ID

	var evt *nostr.Event
	switch action {
	case "reply":
		content := strings.TrimSpace(r.FormValue("content"))
		if content == "" || len(content) > 5000 {
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
		evt = publish.ReplyEvent(eventsSlug, et.ID, content, a.Now())
	case "react":
		emoji := strings.TrimSpace(r.FormValue("emoji"))
		if emoji == "" || len(emoji) > 8 {
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
		evt = publish.ReactionEvent(eventsSlug, et.ID, emoji, a.Now())
	case "rsvp":
		status := r.FormValue("status")
		if status != "accepted" && status != "tentative" && status != "declined" {
			http.NotFound(w, r)
			return
		}
		evt = publish.RSVPEvent(eventsSlug, et.ID, status, a.Now())
	case "approve", "decline", "cancel":
		isAdmin := a.isAdmin(c, viewer.ID)
		holds, _ := c.HasAnyRole(viewer.ID, ch.ApproveRoles)
		canModerate := isAdmin || holds
		switch action {
		case "approve":
			if !canModerate || viewer.Pubkey == et.AuthorPK {
				http.Redirect(w, r, path+"?err=own", http.StatusSeeOther)
				return
			}
			community, _ := a.communityIdentity(c)
			evt = publish.ApprovalEvent(eventsSlug, et.raw, community.Pubkey, c.Slug, a.Now())
		case "decline":
			if !canModerate {
				http.NotFound(w, r)
				return
			}
			evt = publish.DeclineEvent(eventsSlug, et.ID, strings.TrimSpace(r.FormValue("reason")), a.Now())
		case "cancel":
			// The author or an approver may cancel.
			if !canModerate && viewer.Pubkey != et.AuthorPK {
				http.NotFound(w, r)
				return
			}
			evt = publish.CancelEvent(eventsSlug, et.ID, a.Now())
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("event action", "action", action, "err", err)
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// --- ICS feeds ---

func (a *App) eventsToVEvents(events []*eventThread, includeMembers bool, host string) []calendar.VEvent {
	var out []calendar.VEvent
	for _, et := range events {
		if et.Status != "approved" {
			continue
		}
		if !includeMembers && et.Visibility != "public" {
			continue
		}
		out = append(out, calendar.VEvent{
			UID: et.ID, Title: et.Title, Location: et.Location,
			Start: et.Start, End: et.End, AllDay: et.AllDay,
			RRule: et.RRule, Cancelled: et.Cancelled,
		})
	}
	return out
}

func (a *App) publicICS(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.eventsChannel(c); !ok {
		http.NotFound(w, r)
		return
	}
	events, err := a.loadEvents(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Write([]byte(calendar.ICS(c.Hostname, a.eventsToVEvents(events, false, c.Hostname))))
}

func (a *App) membersICS(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.eventsChannel(c); !ok {
		http.NotFound(w, r)
		return
	}
	// Token authentication: calendar apps can't send cookies (EVT-11).
	id, err := c.IdentityByFeedToken(r.URL.Query().Get("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ident, err := c.IdentityByID(id)
	if err != nil || !a.memberLevel(c, ident) {
		http.NotFound(w, r)
		return
	}
	events, err := a.loadEvents(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Write([]byte(calendar.ICS(c.Hostname, a.eventsToVEvents(events, true, c.Hostname))))
}

func (a *App) feedRegenerate(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	if _, err := c.FeedToken(viewer.ID, a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/channels/events", http.StatusSeeOther)
}

// upcomingEvents returns approved, non-cancelled events with a future
// occurrence, soonest first (EVT-05).
func (a *App) upcomingEvents(c *store.Community, includeMembers bool) []*eventThread {
	if _, ok := a.eventsChannel(c); !ok {
		return nil
	}
	events, err := a.loadEvents(c)
	if err != nil {
		return nil
	}
	type withNext struct {
		et   *eventThread
		next time.Time
	}
	var ups []withNext
	for _, et := range events {
		if et.Status != "approved" || et.Cancelled {
			continue
		}
		if !includeMembers && et.Visibility != "public" {
			continue
		}
		next := calendar.NextOccurrence(time.Unix(et.Start, 0).UTC(), et.RRule, a.Now())
		if next.IsZero() {
			continue
		}
		et.When = next.Format("Mon Jan 2 15:04")
		ups = append(ups, withNext{et, next})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].next.Before(ups[j].next) })
	var out []*eventThread
	for i, u := range ups {
		if i >= 5 {
			break
		}
		out = append(out, u.et)
	}
	return out
}

func parseEventTime(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.UTC().Unix(), true
	}
	return 0, false
}
