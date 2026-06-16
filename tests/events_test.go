//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/tests/harness"
)

func enableEvents(t *testing.T, h *harness.H) {
	t.Helper()
	resp, err := h.Admin.PostForm(h.Server.URL+"/settings/channels/events", url.Values{
		"enabled": {"1"}, "approve_roles": {"steward"}, "approvals_required": {"1"},
		"default_visibility": {"public"}, "overridable": {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	time.Sleep(300 * time.Millisecond) // group membership commit
}

func createEvent(t *testing.T, h *harness.H, client *http.Client, v url.Values) string {
	t.Helper()
	resp, err := client.PostForm(h.Server.URL+"/channels/events", v)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		return "" // validation error
	}
	want := v.Get("title")
	for _, evt := range h.QueryRelayAs("xavier", nostr.Filter{
		Kinds: []int{31922, 31923}, Tags: nostr.TagMap{"h": []string{"events"}},
	}) {
		if tagOf(evt, "title") == want {
			return evt.ID
		}
	}
	t.Fatalf("event %q not found on the relay", want)
	return ""
}

func tagOf(evt *nostr.Event, key string) string {
	if tag := evt.Tags.GetFirst([]string{key, ""}); tag != nil && len(*tag) > 1 {
		return (*tag)[1]
	}
	return ""
}

func futureUnix(h *harness.H, days int) string {
	return fmt.Sprint(h.Clock.Now().AddDate(0, 0, days).Unix())
}

func eventForm(h *harness.H, title string, extra url.Values) url.Values {
	v := url.Values{
		"title": {title}, "content": {"Join us."},
		"start": {futureUnix(h, 7)}, "end": {futureUnix(h, 8)},
	}
	for k, vals := range extra {
		v[k] = vals
	}
	return v
}

// TestEVT01_MemberCreatesAnEvent pins EVT-01.
func TestEVT01_MemberCreatesAnEvent(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	id := createEvent(t, h, dan, eventForm(h, "Community call", url.Values{
		"location": {"jitsi.example.org"}, "repeats": {"weekly"},
	}))

	c := h.Community()
	danIdent, _ := c.IdentityByUsername("dan")
	evt := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})[0]
	if evt.Kind != 31923 || evt.PubKey != danIdent.Pubkey {
		t.Fatal("a timed event must be a kind 31923 signed by the author")
	}
	if tagOf(evt, "start") == "" || tagOf(evt, "location") != "jitsi.example.org" {
		t.Fatal("NIP-52 start/location tags must be present")
	}
	if tagOf(evt, "rrule") != "FREQ=WEEKLY" {
		t.Fatalf("rrule must be set, got %q", tagOf(evt, "rrule"))
	}
	if page := body(t, mustGet(t, dan, h.Server.URL+"/channels/events")); !strings.Contains(page, "Community call") || !strings.Contains(page, "pending") {
		t.Fatal("the event must list as pending")
	}
}

// TestEVT02_TemplateValidation pins EVT-02.
func TestEVT02_TemplateValidation(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")

	// End before start is rejected.
	resp, _ := dan.PostForm(h.Server.URL+"/channels/events", url.Values{
		"title": {"Backwards"}, "start": {futureUnix(h, 8)}, "end": {futureUnix(h, 7)},
	})
	if page := body(t, resp); !strings.Contains(page, "end after it starts") {
		t.Fatal("end-before-start must be rejected")
	}
	// All-day → kind 31922.
	id := createEvent(t, h, dan, url.Values{
		"title": {"Festival"}, "start": {futureUnix(h, 7)}, "end": {futureUnix(h, 8)},
		"all_day": {"1"},
	})
	if h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})[0].Kind != 31922 {
		t.Fatal("an all-day event must be kind 31922")
	}
}

// TestEVT03_ApprovalMakesItPublic pins EVT-03.
func TestEVT03_ApprovalMakesItPublic(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	id := createEvent(t, h, dan, eventForm(h, "Public meetup", nil))

	// Hidden from visitors while pending.
	if strings.Contains(body(t, h.Get("/channels/events")), "Public meetup") {
		t.Fatal("a pending event must be hidden from visitors")
	}
	resp, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
	resp.Body.Close()
	if !strings.Contains(body(t, h.Get("/channels/events")), "Public meetup") {
		t.Fatal("an approved public event must be visible to visitors")
	}
}

// TestEVT04_PublicICSFeed pins EVT-04 and the ICS half of EVT-09.
func TestEVT04_PublicICSFeed(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")

	pub := createEvent(t, h, dan, eventForm(h, "Recurring call", url.Values{"repeats": {"weekly"}}))
	members := createEvent(t, h, dan, eventForm(h, "Internal sync", url.Values{"visibility": {"members"}}))
	_ = createEvent(t, h, dan, eventForm(h, "Unapproved one", nil))

	for _, id := range []string{pub, members} {
		resp, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
		resp.Body.Close()
	}

	resp := h.Get("/channels/events/public.ics")
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/calendar") {
		t.Fatalf("ICS content type: %s", ct)
	}
	feed := body(t, resp)
	if !strings.Contains(feed, "SUMMARY:Recurring call") || !strings.Contains(feed, "RRULE:FREQ=WEEKLY") {
		t.Fatal("the public feed must carry approved public events with their RRULE")
	}
	if !strings.Contains(feed, "DTSTART") || !strings.Contains(feed, "DTEND") {
		t.Fatal("VEVENTs must carry DTSTART/DTEND")
	}
	if strings.Contains(feed, "Internal sync") || strings.Contains(feed, "Unapproved one") {
		t.Fatal("members-only and pending events must be absent from the public feed")
	}
}

// TestEVT05_HomepageUpcomingIsConditionalAndViewerAware pins EVT-05.
func TestEVT05_HomepageUpcomingIsConditionalAndViewerAware(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)

	// No events channel → no section.
	if strings.Contains(body(t, h.Get("/")), "Upcoming events") {
		t.Fatal("no events section before the channel is enabled")
	}
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	id := createEvent(t, h, dan, eventForm(h, "Members evening", url.Values{"visibility": {"members"}}))
	resp, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
	resp.Body.Close()

	// A members-only event shows to members, not visitors.
	if strings.Contains(body(t, h.Get("/")), "Members evening") {
		t.Fatal("a members-only event must not show to visitors")
	}
	if !strings.Contains(body(t, mustGet(t, dan, h.Server.URL+"/")), "Members evening") {
		t.Fatal("a members-only event must show on the member homepage")
	}
}

// TestEVT06_CancellingAnEvent pins EVT-06.
func TestEVT06_CancellingAnEvent(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	id := createEvent(t, h, dan, eventForm(h, "Doomed event", nil))
	resp, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
	resp.Body.Close()

	if !strings.Contains(body(t, h.Get("/")), "Doomed event") {
		t.Fatal("the approved event should be on the homepage first")
	}
	// The author cancels.
	resp, _ = dan.PostForm(h.Server.URL+"/channels/events/t/"+id+"/cancel", nil)
	resp.Body.Close()

	if strings.Contains(body(t, h.Get("/")), "Doomed event") {
		t.Fatal("a cancelled event must leave the homepage")
	}
	if !strings.Contains(body(t, h.Get("/channels/events/public.ics")), "STATUS:CANCELLED") {
		t.Fatal("a cancelled event must be marked CANCELLED in the feed")
	}
}

// TestEVT08_RSVP pins EVT-08.
func TestEVT08_RSVP(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	id := createEvent(t, h, dan, eventForm(h, "RSVP test", nil))
	resp, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
	resp.Body.Close()

	rsvp := func(cl *http.Client, status string) {
		r, _ := cl.PostForm(h.Server.URL+"/channels/events/t/"+id+"/rsvp", url.Values{"status": {status}})
		r.Body.Close()
	}
	rsvp(alice, "accepted")
	page := body(t, mustGet(t, dan, h.Server.URL+"/channels/events/t/"+id))
	if !strings.Contains(page, "@alice") || !strings.Contains(page, "(1)") {
		t.Fatal("alice's 'going' RSVP must show")
	}
	// Changing replaces (no duplicate): alice → interested.
	rsvp(alice, "tentative")
	page = body(t, mustGet(t, dan, h.Server.URL+"/channels/events/t/"+id))
	if strings.Contains(page, "going: @alice") {
		t.Fatal("changing the RSVP must replace the prior one")
	}
	// Visitors cannot RSVP.
	r, _ := h.Client().PostForm(h.Server.URL+"/channels/events/t/"+id+"/rsvp", url.Values{"status": {"accepted"}})
	r.Body.Close()
	if r.StatusCode == 303 {
		t.Fatal("a visitor must not be able to RSVP")
	}
}

// TestEVT10_LocationExternalCover pins EVT-10.
func TestEVT10_LocationExternalCover(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")

	id := createEvent(t, h, dan, eventForm(h, "Rich event", url.Values{
		"location": {"Brussels"}, "external": {"https://lu.ma/x"}, "image": {"https://blossom/cover.jpg"},
	}))
	evt := h.QueryRelayAs("dan", nostr.Filter{IDs: []string{id}})[0]
	if tagOf(evt, "location") != "Brussels" || tagOf(evt, "r") != "https://lu.ma/x" || tagOf(evt, "image") == "" {
		t.Fatal("location, external link and cover must be on the root")
	}
	// Non-http(s) external URL is rejected.
	resp, _ := dan.PostForm(h.Server.URL+"/channels/events", eventForm(h, "Bad link", url.Values{
		"external": {"javascript:alert(1)"},
	}))
	if page := body(t, resp); !strings.Contains(page, "http(s) URL") {
		t.Fatal("a non-http(s) external link must be rejected")
	}
}

// TestEVT11_MembersICSTokenAuthenticated pins EVT-11.
func TestEVT11_MembersICSTokenAuthenticated(t *testing.T) {
	h := harness.New(t)
	h.CompleteSetup(false)
	requireZooid(t, h)
	enableEvents(t, h)
	dan := h.Member("dan")
	alice := h.Member("alice", "steward")
	pub := createEvent(t, h, dan, eventForm(h, "Public party", nil))
	mem := createEvent(t, h, dan, eventForm(h, "Members meeting", url.Values{"visibility": {"members"}}))
	for _, id := range []string{pub, mem} {
		r, _ := alice.PostForm(h.Server.URL+"/channels/events/t/"+id+"/approve", nil)
		r.Body.Close()
	}

	token := memberFeedToken(t, h, dan)
	feed := body(t, mustGet(t, h.Client(), h.Server.URL+"/channels/events/members.ics?token="+token))
	if !strings.Contains(feed, "Public party") || !strings.Contains(feed, "Members meeting") {
		t.Fatal("the members feed must contain both visibilities")
	}
	// Missing / wrong token → not-found.
	for _, tok := range []string{"", "deadbeef"} {
		r, _ := h.Client().Get(h.Server.URL + "/channels/events/members.ics?token=" + tok)
		r.Body.Close()
		if r.StatusCode != 404 {
			t.Fatalf("token %q must 404, got %d", tok, r.StatusCode)
		}
	}
	// Regenerating invalidates the old token.
	r, _ := dan.PostForm(h.Server.URL+"/channels/events/subscribe", nil)
	r.Body.Close()
	r2, _ := h.Client().Get(h.Server.URL + "/channels/events/members.ics?token=" + token)
	r2.Body.Close()
	if r2.StatusCode != 404 {
		t.Fatal("a regenerated token must invalidate the old URL")
	}
}

func memberFeedToken(t *testing.T, h *harness.H, client *http.Client) string {
	t.Helper()
	page := body(t, mustGet(t, client, h.Server.URL+"/channels/events"))
	i := strings.Index(page, "members.ics?token=")
	if i < 0 {
		t.Fatal("no members feed link on the events page")
	}
	rest := page[i+len("members.ics?token="):]
	end := strings.IndexAny(rest, `"<`)
	return rest[:end]
}
