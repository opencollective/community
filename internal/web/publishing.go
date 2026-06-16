package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/markdown"
	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// Publishing as the community (docs/nostr/publishing.md): members with
// propose_posts draft announcements / blog posts / newsletters; on the
// section's approval quorum the bunker signs the final event with the
// community key. The trail is signed relay events; the app holds only an
// at-most-once newsletter guard.

// publishMu serializes publish-on-quorum within the process; correctness
// across restarts/DB-wipes rests on the relay-existence check (PUB-12).
var publishMu sync.Mutex

var contentTypes = map[string]string{
	"announcement": "Announcement",
	"blog":         "Blog post",
	"newsletter":   "Newsletter",
}

// requireProposer gates /compose on propose_posts (or admin).
func (a *App) requireProposer(h func(http.ResponseWriter, *http.Request, *store.Community, *store.Identity)) http.HandlerFunc {
	return a.requireMember(func(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
		ok := a.isAdmin(c, viewer.ID)
		if !ok {
			ok, _ = c.HasPermission(viewer.ID, store.PermProposePosts)
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r, c, viewer)
	})
}

func (a *App) composeForm(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	a.renderCompose(w, "announcement", "", "", "")
}

func (a *App) renderCompose(w http.ResponseWriter, ctype, title, content, errMsg string) {
	a.render(w, "compose.html", map[string]any{
		"Title": "New post", "ContentType": ctype, "FormTitle": title,
		"Content": content, "Error": errMsg,
	})
}

func (a *App) composeSubmit(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	ctype := r.FormValue("content_type")
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	if _, ok := contentTypes[ctype]; !ok {
		a.renderCompose(w, "announcement", title, content, "Pick a post type.")
		return
	}
	if ctype != "announcement" && title == "" {
		a.renderCompose(w, ctype, title, content, "A title is required.")
		return
	}
	if content == "" || len(content) > 50000 {
		a.renderCompose(w, ctype, title, content, "Write something (under 50k characters).")
		return
	}

	community, err := a.communityIdentity(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	p, ok := a.publisher(c)
	if !ok {
		a.renderCompose(w, ctype, title, content, "The relay is not available.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	uniqueD := randomD()
	evt := publish.ProposalEvent(ctype, title, content, community.Pubkey, c.Slug, uniqueD, a.Now())
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("proposal publish", "err", err)
		a.renderCompose(w, ctype, title, content, "Could not submit — try again.")
		return
	}
	http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
}

// proposal is the assembled state of one pending/decided post.
type proposal struct {
	ID          string
	ContentType string
	TypeLabel   string
	Title       string
	Content     string
	Author      string
	AuthorPK    string
	Status      string // pending | approved | declined
	Time        string
	Approvers   []string
	Required    int
	DeclinedBy  string
	Reason      string
	raw         *nostr.Event
}

// loadProposals reads every proposal, approval, decline and community
// publication, and assembles status (PUB-02/03/12).
func (a *App) loadProposals(c *store.Community) ([]*proposal, map[string]bool, error) {
	p, ok := a.publisher(c)
	if !ok {
		return nil, nil, nil
	}
	community, err := a.communityIdentity(c)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return nil, nil, err
	}
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{
		Kinds: []int{publish.KindAnnouncement, publish.KindArticle,
			publish.KindApproval, publish.KindLabel},
		Limit: 500,
	})
	if err != nil {
		return nil, nil, err
	}

	// published[proposalID] = true when a community event mentions it.
	published := map[string]bool{}
	var proposals []*proposal
	var approvals, declines []*nostr.Event
	for _, evt := range events {
		switch evt.Kind {
		case publish.KindApproval:
			approvals = append(approvals, evt)
		case publish.KindLabel:
			declines = append(declines, evt)
		case publish.KindAnnouncement, publish.KindArticle:
			if evt.PubKey == community.Pubkey {
				if ref := mentionedProposal(evt); ref != "" {
					published[ref] = true
				}
				continue
			}
			if tag := evt.Tags.GetFirst([]string{"proposal", ""}); tag != nil && len(*tag) > 1 {
				pr := &proposal{
					ID: evt.ID, ContentType: (*tag)[1], Content: evt.Content,
					AuthorPK: evt.PubKey, Status: "pending", Time: formatTime(evt.CreatedAt),
					raw: evt,
				}
				pr.TypeLabel = contentTypes[pr.ContentType]
				if t := evt.Tags.GetFirst([]string{"title", ""}); t != nil && len(*t) > 1 {
					pr.Title = (*t)[1]
				}
				if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
					pr.Author = ident.Username
				} else {
					pr.Author = evt.PubKey[:8] + "…"
				}
				if pol, err := c.PostPolicyFor(policyKey(pr.ContentType)); err == nil {
					pr.Required = pol.ApprovalsRequired
				}
				proposals = append(proposals, pr)
			}
		}
	}

	byID := map[string]*proposal{}
	for _, pr := range proposals {
		byID[pr.ID] = pr
	}
	for _, evt := range approvals {
		ref := firstETag(evt)
		pr := byID[ref]
		if pr == nil || evt.PubKey == pr.AuthorPK {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		pol, err := c.PostPolicyFor(policyKey(pr.ContentType))
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, pol.ApproveRoles)
		if !isAdmin && !holds {
			continue
		}
		dup := false
		for _, u := range pr.Approvers {
			if u == ident.Username {
				dup = true
			}
		}
		if !dup {
			pr.Approvers = append(pr.Approvers, ident.Username)
		}
		if isAdmin || len(pr.Approvers) >= pol.ApprovalsRequired {
			pr.Status = "approved"
		}
	}
	for _, evt := range declines {
		if evt.Tags.GetFirst([]string{"l", "declined"}) == nil {
			continue
		}
		pr := byID[firstETag(evt)]
		if pr == nil || pr.Status == "approved" {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		pol, _ := c.PostPolicyFor(policyKey(pr.ContentType))
		isAdmin := a.isAdmin(c, ident.ID)
		holds := false
		if pol != nil {
			holds, _ = c.HasAnyRole(ident.ID, pol.ApproveRoles)
		}
		if !isAdmin && !holds {
			continue
		}
		pr.Status = "declined"
		pr.DeclinedBy = ident.Username
		pr.Reason = evt.Content
	}

	sort.Slice(proposals, func(i, j int) bool {
		return proposals[i].raw.CreatedAt > proposals[j].raw.CreatedAt
	})
	return proposals, published, nil
}

func (a *App) pendingPosts(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	proposals, published, err := a.loadProposals(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	canDecide, _ := c.HasPermission(viewer.ID, store.PermApprovePosts)
	if a.isAdmin(c, viewer.ID) {
		canDecide = true
	}
	var pending []*proposal
	for _, pr := range proposals {
		if published[pr.ID] || pr.Status == "declined" {
			continue
		}
		pending = append(pending, pr)
	}
	profileEdits, err := a.loadProfileEdits(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.render(w, "posts_pending.html", map[string]any{
		"Title": "Pending posts", "Proposals": pending, "ProfileEdits": profileEdits,
		"CanDecide": canDecide, "Error": r.URL.Query().Get("err"),
	})
}

func (a *App) postsDecide(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	if action != "approve" && action != "decline" {
		http.NotFound(w, r)
		return
	}
	canApprove, _ := c.HasPermission(viewer.ID, store.PermApprovePosts)
	isAdmin := a.isAdmin(c, viewer.ID)
	if !isAdmin && !canApprove {
		http.NotFound(w, r)
		return
	}
	proposals, published, err := a.loadProposals(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	var pr *proposal
	for _, p := range proposals {
		if p.ID == id {
			pr = p
		}
	}
	if pr == nil || published[pr.ID] || pr.Status != "pending" {
		http.NotFound(w, r)
		return
	}
	if viewer.Pubkey == pr.AuthorPK {
		http.Redirect(w, r, "/posts/pending?err=own", http.StatusSeeOther)
		return
	}

	community, err := a.communityIdentity(c)
	if err != nil {
		a.internalError(w, err)
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

	var evt *nostr.Event
	if action == "approve" {
		evt = publish.ApprovalEvent("", pr.raw, community.Pubkey, c.Slug, a.Now())
	} else {
		evt = publish.DeclineEvent("", pr.ID, strings.TrimSpace(r.FormValue("reason")), a.Now())
	}
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("post decision", "action", action, "err", err)
		http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
		return
	}

	if action == "approve" {
		a.maybePublish(c, pr.ID)
	}
	http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
}

// maybePublish signs and publishes the community event when the proposal's
// quorum is met and it is not already published (idempotent via the
// relay-existence check).
func (a *App) maybePublish(c *store.Community, proposalID string) {
	publishMu.Lock()
	defer publishMu.Unlock()

	proposals, published, err := a.loadProposals(c)
	if err != nil {
		a.Log.Error("maybePublish load", "err", err)
		return
	}
	if published[proposalID] {
		return
	}
	var pr *proposal
	for _, p := range proposals {
		if p.ID == proposalID {
			pr = p
		}
	}
	if pr == nil || pr.Status != "approved" {
		return
	}

	community, err := a.communityIdentity(c)
	if err != nil {
		return
	}
	p, ok := a.publisher(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(a.baseCtx, publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		return
	}
	slug := ""
	if pr.ContentType != "announcement" {
		slug = markdown.Slug(pr.Title) + "-" + shortID(pr.ID)
	}
	evt := publish.PublishedEvent(pr.ContentType, pr.Title, pr.Content, slug,
		pr.AuthorPK, pr.ID, a.Now())
	if err := p.PublishAs(ctx, community, claim, evt); err != nil {
		a.Log.Error("community publish", "type", pr.ContentType, "err", err)
		return
	}
	if pr.ContentType == "newsletter" {
		a.sendNewsletter(c, evt, pr.Title)
	}
}

// --- public rendering ---

// communityPosts queries the community's published events of a kind.
func (a *App) communityPosts(c *store.Community, kinds []int, limit int) []*nostr.Event {
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
	events, err := p.QueryAs(ctx, community, claim, nostr.Filter{
		Authors: []string{community.Pubkey}, Kinds: kinds, Limit: limit,
	})
	if err != nil {
		return nil
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt > events[j].CreatedAt })
	return events
}

type feedItem struct {
	Title   string
	Slug    string
	Summary string
	Content string
	HTML    string
	Date    string
	IsNews  bool
}

// homeFeed assembles recent announcements and blog posts for the homepage.
func (a *App) homeFeed(c *store.Community) (announcements []feedItem, blog []feedItem) {
	for _, evt := range a.communityPosts(c, []int{publish.KindAnnouncement}, 5) {
		announcements = append(announcements, feedItem{
			Content: evt.Content, Date: formatTime(evt.CreatedAt),
		})
	}
	for _, evt := range a.communityPosts(c, []int{publish.KindArticle}, 10) {
		if publish.IsNewsletter(evt) {
			continue
		}
		blog = append(blog, articleItem(evt))
		if len(blog) >= 5 {
			break
		}
	}
	return
}

func articleItem(evt *nostr.Event) feedItem {
	it := feedItem{Content: evt.Content, Date: formatTime(evt.CreatedAt), IsNews: publish.IsNewsletter(evt)}
	if t := evt.Tags.GetFirst([]string{"title", ""}); t != nil && len(*t) > 1 {
		it.Title = (*t)[1]
	}
	if d := evt.Tags.GetFirst([]string{"d", ""}); d != nil && len(*d) > 1 {
		it.Slug = (*d)[1]
	}
	it.Summary = summarize(evt.Content)
	return it
}

func (a *App) postPage(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	slug := r.PathValue("slug")
	for _, evt := range a.communityPosts(c, []int{publish.KindArticle}, 50) {
		d := evt.Tags.GetFirst([]string{"d", ""})
		if d == nil || (*d)[1] != slug {
			continue
		}
		it := articleItem(evt)
		a.render(w, "post.html", map[string]any{
			"Title": it.Title, "Post": it,
			"HTML": template.HTML(markdown.HTML(evt.Content)),
		})
		return
	}
	http.NotFound(w, r)
}

// rssFeed serves blog posts as RSS (PUB-14): published blog articles only,
// no pending content, no newsletters.
func (a *App) rssFeed(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	name, _ := c.Setting(setName)
	desc, _ := c.Setting(setDescription)
	base := a.publicURL(c, "http", "")
	if !a.DevMode {
		base = "https://" + c.Hostname
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0"><channel>` + "\n")
	b.WriteString("<title>" + xmlEscape(name) + "</title>\n")
	b.WriteString("<link>" + xmlEscape(base) + "</link>\n")
	b.WriteString("<description>" + xmlEscape(desc) + "</description>\n")
	for _, evt := range a.communityPosts(c, []int{publish.KindArticle}, 50) {
		if publish.IsNewsletter(evt) {
			continue
		}
		it := articleItem(evt)
		link := base + "/posts/" + it.Slug
		b.WriteString("<item>")
		b.WriteString("<title>" + xmlEscape(it.Title) + "</title>")
		b.WriteString("<link>" + xmlEscape(link) + "</link>")
		b.WriteString("<guid>" + xmlEscape(link) + "</guid>")
		b.WriteString("<description>" + xmlEscape(it.Summary) + "</description>")
		b.WriteString("</item>\n")
	}
	b.WriteString("</channel></rss>\n")

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Write([]byte(b.String()))
}

// --- helpers ---

func mentionedProposal(evt *nostr.Event) string {
	for _, tag := range evt.Tags.GetAll([]string{"e", ""}) {
		if len(tag) >= 4 && tag[3] == "mention" {
			return tag[1]
		}
	}
	return ""
}

func policyKey(contentType string) string {
	if contentType == "announcement" {
		return "announcement"
	}
	if contentType == "newsletter" {
		return "newsletter"
	}
	return "blog"
}

func randomD() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func summarize(content string) string {
	s := markdown.Text(content)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
