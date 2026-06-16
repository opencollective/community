package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/publish"
	"github.com/opencollective/community/internal/store"
)

// Community profile edits and the homepage linktree
// (docs/nostr/publishing.md § profile edits). Any member may propose a
// change to name / description / icon / links; approval reuses the
// publishing quorum (approve_posts), and on quorum the bunker builds the
// new community kind 0 from the approved JSON.

const setLinks = "links"

// currentProfile reads the community's current editable profile from the
// settings render cache (rebuildable from the community kind 0).
func (a *App) currentProfile(c *store.Community) publish.ProfileData {
	name, _ := c.Setting(setName)
	about, _ := c.Setting(setDescription)
	picture, _ := c.Setting(setIcon)
	pd := publish.ProfileData{Name: name, About: about, Picture: picture}
	if raw, err := c.Setting(setLinks); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &pd.Links)
	}
	return pd
}

func profileHash(pd publish.ProfileData) string {
	b, _ := json.Marshal(pd)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (a *App) profileEditForm(w http.ResponseWriter, r *http.Request, c *store.Community, _ *store.Identity) {
	a.renderProfileEdit(w, c, a.currentProfile(c), "")
}

func (a *App) renderProfileEdit(w http.ResponseWriter, c *store.Community, pd publish.ProfileData, errMsg string) {
	a.render(w, "profile_edit.html", map[string]any{
		"Title": "Propose a profile change", "P": pd, "Error": errMsg,
	})
}

func (a *App) profileEditSubmit(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	pd := publish.ProfileData{
		Name:    strings.TrimSpace(r.FormValue("name")),
		About:   strings.TrimSpace(r.FormValue("about")),
		Picture: strings.TrimSpace(r.FormValue("picture")),
	}
	labels := r.Form["link_label"]
	urls := r.Form["link_url"]
	for i := range labels {
		label := strings.TrimSpace(labels[i])
		u := ""
		if i < len(urls) {
			u = strings.TrimSpace(urls[i])
		}
		if label == "" && u == "" {
			continue
		}
		pd.Links = append(pd.Links, publish.ProfileLink{Label: label, URL: u})
	}

	if err := validateProfile(pd); err != "" {
		a.renderProfileEdit(w, c, pd, err)
		return
	}
	if pd.Picture != "" && !okURL(pd.Picture) {
		a.renderProfileEdit(w, c, pd, "The icon must be an http(s) URL.")
		return
	}

	community, err := a.communityIdentity(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	p, ok := a.publisher(c)
	if !ok {
		a.renderProfileEdit(w, c, pd, "The relay is not available.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	claim, err := a.claim(ctx, c, p)
	if err != nil {
		a.internalError(w, err)
		return
	}
	profileJSON, _ := json.Marshal(pd)
	base := profileHash(a.currentProfile(c))
	evt := publish.ProfileEditWrapper(string(profileJSON), community.Pubkey, c.Slug,
		randomD(), base, a.Now())
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("profile edit publish", "err", err)
		a.renderProfileEdit(w, c, pd, "Could not submit — try again.")
		return
	}
	http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
}

// validateProfile enforces the whitelist limits (PROF-05).
func validateProfile(pd publish.ProfileData) string {
	if pd.Name == "" {
		return "A community name is required."
	}
	if len(pd.Links) > 20 {
		return "Too many links (max 20)."
	}
	for _, l := range pd.Links {
		if l.URL == "" || !okURL(l.URL) {
			return "Every link needs an http(s) URL."
		}
	}
	return ""
}

func okURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

type profileDiffRow struct{ Field, Old, New string }

type profileEdit struct {
	ID        string
	Author    string
	AuthorPK  string
	Status    string
	Time      string
	Approvers []string
	Required  int
	Stale     bool
	Diff      []profileDiffRow
	raw       *nostr.Event
}

// loadProfileEdits assembles pending profile-edit proposals with their
// diffs and status (PROF-02/06/08).
func (a *App) loadProfileEdits(c *store.Community) ([]*profileEdit, error) {
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
		Kinds: []int{publish.KindAppData, publish.KindApproval, publish.KindLabel},
		Limit: 500,
	})
	if err != nil {
		return nil, err
	}
	applied, err := c.AppliedProfileEdits()
	if err != nil {
		return nil, err
	}
	pol, err := c.PostPolicyFor("profile")
	if err != nil {
		return nil, err
	}
	current := a.currentProfile(c)
	currentHash := profileHash(current)

	var wrappers []*nostr.Event
	var approvals, declines []*nostr.Event
	for _, evt := range events {
		switch evt.Kind {
		case publish.KindAppData:
			if t := evt.Tags.GetFirst([]string{"proposal", "profile"}); t != nil {
				wrappers = append(wrappers, evt)
			}
		case publish.KindApproval:
			approvals = append(approvals, evt)
		case publish.KindLabel:
			declines = append(declines, evt)
		}
	}

	byID := map[string]*profileEdit{}
	var out []*profileEdit
	for _, evt := range wrappers {
		if applied[evt.ID] {
			continue
		}
		var pd publish.ProfileData
		if err := json.Unmarshal([]byte(evt.Content), &pd); err != nil {
			continue
		}
		pe := &profileEdit{
			ID: evt.ID, AuthorPK: evt.PubKey, Status: "pending",
			Time: formatTime(evt.CreatedAt), Required: pol.ApprovalsRequired,
			Diff: diffProfile(current, pd), raw: evt,
		}
		if base := evt.Tags.GetFirst([]string{"base", ""}); base != nil && len(*base) > 1 {
			pe.Stale = (*base)[1] != currentHash
		}
		if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
			pe.Author = ident.Username
		} else {
			pe.Author = evt.PubKey[:8] + "…"
		}
		byID[evt.ID] = pe
		out = append(out, pe)
	}

	for _, evt := range approvals {
		pe := byID[firstETag(evt)]
		if pe == nil || evt.PubKey == pe.AuthorPK {
			continue
		}
		ident, err := c.IdentityByPubkey(evt.PubKey)
		if err != nil {
			continue
		}
		isAdmin := a.isAdmin(c, ident.ID)
		holds, _ := c.HasAnyRole(ident.ID, pol.ApproveRoles)
		if !isAdmin && !holds {
			continue
		}
		dup := false
		for _, u := range pe.Approvers {
			if u == ident.Username {
				dup = true
			}
		}
		if !dup {
			pe.Approvers = append(pe.Approvers, ident.Username)
		}
		if isAdmin || len(pe.Approvers) >= pol.ApprovalsRequired {
			pe.Status = "approved"
		}
	}
	for _, evt := range declines {
		if evt.Tags.GetFirst([]string{"l", "declined"}) == nil {
			continue
		}
		pe := byID[firstETag(evt)]
		if pe == nil || pe.Status == "approved" {
			continue
		}
		if ident, err := c.IdentityByPubkey(evt.PubKey); err == nil {
			isAdmin := a.isAdmin(c, ident.ID)
			holds, _ := c.HasAnyRole(ident.ID, pol.ApproveRoles)
			if isAdmin || holds {
				pe.Status = "declined"
			}
		}
	}

	var pending []*profileEdit
	for _, pe := range out {
		if pe.Status != "declined" {
			pending = append(pending, pe)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].raw.CreatedAt > pending[j].raw.CreatedAt
	})
	return pending, nil
}

func diffProfile(old, neu publish.ProfileData) []profileDiffRow {
	var rows []profileDiffRow
	if old.Name != neu.Name {
		rows = append(rows, profileDiffRow{"name", old.Name, neu.Name})
	}
	if old.About != neu.About {
		rows = append(rows, profileDiffRow{"description", old.About, neu.About})
	}
	if old.Picture != neu.Picture {
		rows = append(rows, profileDiffRow{"icon", old.Picture, neu.Picture})
	}
	if linksStr(old.Links) != linksStr(neu.Links) {
		rows = append(rows, profileDiffRow{"links", linksStr(old.Links), linksStr(neu.Links)})
	}
	return rows
}

func linksStr(links []publish.ProfileLink) string {
	var parts []string
	for _, l := range links {
		parts = append(parts, l.Label+" → "+l.URL)
	}
	return strings.Join(parts, ", ")
}

func (a *App) profileDecide(w http.ResponseWriter, r *http.Request, c *store.Community, viewer *store.Identity) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	if action != "approve" && action != "decline" {
		http.NotFound(w, r)
		return
	}
	canApprove, _ := c.HasPermission(viewer.ID, store.PermApprovePosts)
	if !a.isAdmin(c, viewer.ID) && !canApprove {
		http.NotFound(w, r)
		return
	}
	edits, err := a.loadProfileEdits(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	var pe *profileEdit
	for _, e := range edits {
		if e.ID == id {
			pe = e
		}
	}
	if pe == nil || pe.Status != "pending" {
		http.NotFound(w, r)
		return
	}
	if viewer.Pubkey == pe.AuthorPK {
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
		evt = publish.ApprovalEvent("", pe.raw, community.Pubkey, c.Slug, a.Now())
	} else {
		evt = publish.DeclineEvent("", pe.ID, strings.TrimSpace(r.FormValue("reason")), a.Now())
	}
	if err := p.PublishAs(ctx, viewer, claim, evt); err != nil {
		a.Log.Error("profile decision", "err", err)
		http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
		return
	}
	if action == "approve" {
		a.maybePublishProfile(c, id)
	}
	http.Redirect(w, r, "/posts/pending", http.StatusSeeOther)
}

// maybePublishProfile signs the new community kind 0 from an approved
// wrapper and syncs the render cache (PROF-03).
func (a *App) maybePublishProfile(c *store.Community, wrapperID string) {
	publishMu.Lock()
	defer publishMu.Unlock()

	edits, err := a.loadProfileEdits(c)
	if err != nil {
		return
	}
	var pe *profileEdit
	for _, e := range edits {
		if e.ID == wrapperID {
			pe = e
		}
	}
	if pe == nil || pe.Status != "approved" {
		return
	}
	var pd publish.ProfileData
	if err := json.Unmarshal([]byte(pe.raw.Content), &pd); err != nil {
		return
	}
	claimed, err := c.ClaimProfileEditApplied(wrapperID, a.Now())
	if err != nil || !claimed {
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
	metadata, _ := json.Marshal(pd)
	evt := publish.RawProfileEvent(string(metadata), a.Now())
	if err := p.PublishAs(ctx, community, claim, evt); err != nil {
		a.Log.Error("community profile publish", "err", err)
		return
	}
	// Sync the render cache. Newsletter is never triggered (PROF-03).
	_ = c.SetSetting(setName, pd.Name)
	_ = c.SetSetting(setDescription, pd.About)
	_ = c.SetSetting(setIcon, pd.Picture)
	links, _ := json.Marshal(pd.Links)
	_ = c.SetSetting(setLinks, string(links))
}
