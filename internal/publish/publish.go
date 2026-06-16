// Package publish writes identities' events to the community's data relay
// (zooid): NIP-42 auth, relay membership joins with invite claims, and the
// profile / follow / community-definition events of docs/nostr.
package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/bunker"
	"github.com/opencollective/community/internal/store"
)

const (
	kindRelayJoin   = 28934
	kindRelayInvite = 28935
	// KindCommunityDefinition is the NIP-72 kind 34550.
	KindCommunityDefinition = 34550
)

// Client talks to one community's data relay on behalf of its identities.
type Client struct {
	URL    string
	Signer *bunker.Signer
}

// session is one authed relay connection for an identity.
type session struct {
	relay *nostr.Relay
	ident *store.Identity
	c     *Client
}

func (c *Client) connect(ctx context.Context, ident *store.Identity) (*session, error) {
	// Retry the websocket handshake: a freshly-provisioned community's
	// virtual relay appears only once zooid's inotify picks up the config
	// communityd just wrote, so the first connect can race that reload.
	var lastErr error
	for i := 0; i < 30; i++ {
		relay, err := nostr.RelayConnect(ctx, c.URL)
		if err == nil {
			return &session{relay: relay, ident: ident, c: c}, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("publish: connect %s: %w", c.URL, lastErr)
}

func (s *session) close() { s.relay.Close() }

// auth answers the relay's NIP-42 challenge. The challenge arrives
// asynchronously right after connect, so early attempts may race it —
// retry briefly until the relay accepts.
func (s *session) auth(ctx context.Context) error {
	sign := func(evt *nostr.Event) error {
		return s.c.Signer.SignAs(s.ident, evt)
	}
	var err error
	for i := 0; i < 40; i++ {
		err = s.relay.Auth(ctx, sign)
		if err == nil || !strings.Contains(err.Error(), "challenge") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return err
}

// publish signs and publishes, transparently answering an AUTH challenge.
func (s *session) publish(ctx context.Context, evt *nostr.Event) error {
	if err := s.c.Signer.SignAs(s.ident, evt); err != nil {
		return err
	}
	err := s.relay.Publish(ctx, *evt)
	if err != nil && strings.Contains(err.Error(), "auth-required") {
		if err := s.auth(ctx); err != nil {
			return err
		}
		// Re-sign: zooid may reject replays of the same id after auth.
		err = s.relay.Publish(ctx, *evt)
	}
	return err
}

// join makes the identity a relay member (zooid kind 28934). The relay is
// invite-only; claim comes from the community's standing invite.
func (s *session) join(ctx context.Context, claim string) error {
	tags := nostr.Tags{}
	if claim != "" {
		tags = append(tags, nostr.Tag{"claim", claim})
	}
	evt := &nostr.Event{Kind: kindRelayJoin, Tags: tags}
	if err := s.auth(ctx); err != nil {
		return err
	}
	return s.publish(ctx, evt)
}

// Claim fetches the relay's standing invite code as the community
// identity, whose can_invite role in the zooid config authorizes the read.
func (c *Client) Claim(ctx context.Context, community *store.Identity) (string, error) {
	s, err := c.connect(ctx, community)
	if err != nil {
		return "", err
	}
	defer s.close()
	if err := s.auth(ctx); err != nil {
		return "", err
	}
	events, err := s.relay.QuerySync(ctx, nostr.Filter{Kinds: []int{kindRelayInvite}, Limit: 1})
	if err != nil {
		return "", err
	}
	for _, evt := range events {
		if tag := evt.Tags.GetFirst([]string{"claim", ""}); tag != nil && len(*tag) > 1 {
			return (*tag)[1], nil
		}
	}
	return "", fmt.Errorf("publish: no invite available")
}

// PublishAs joins the identity to the relay (idempotent) and publishes
// the given events, signing each with the identity's key.
func (c *Client) PublishAs(ctx context.Context, ident *store.Identity, claim string, events ...*nostr.Event) error {
	s, err := c.connect(ctx, ident)
	if err != nil {
		return err
	}
	defer s.close()
	if err := s.join(ctx, claim); err != nil {
		return fmt.Errorf("publish: join as %s: %w", ident.Username, err)
	}
	for _, evt := range events {
		if err := s.publish(ctx, evt); err != nil {
			return fmt.Errorf("publish: %s kind %d: %w", ident.Username, evt.Kind, err)
		}
	}
	return nil
}

// QueryAs reads from the relay as an identity (joining first — reads are
// members-only).
func (c *Client) QueryAs(ctx context.Context, ident *store.Identity, claim string, filter nostr.Filter) ([]*nostr.Event, error) {
	s, err := c.connect(ctx, ident)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if err := s.join(ctx, claim); err != nil {
		return nil, err
	}
	return s.relay.QuerySync(ctx, filter)
}

// ProfileEvent builds a kind 0 for an identity.
func ProfileEvent(name, about, picture string, now time.Time) *nostr.Event {
	content := fmt.Sprintf(`{"name":%q,"about":%q,"picture":%q}`, name, about, picture)
	return &nostr.Event{Kind: nostr.KindProfileMetadata, Content: content,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// FollowEvent builds a kind 3 following the community.
func FollowEvent(communityPubkey string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindFollowList,
		Tags:      nostr.Tags{{"p", communityPubkey}},
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// --- NIP-29 group events (docs/nostr/chat.md) ---

// GroupCreateEvent builds a kind 9007 creating group h. Sender needs
// can_manage in the zooid config.
func GroupCreateEvent(h string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindSimpleGroupCreateGroup,
		Tags: nostr.Tags{{"h", h}}, CreatedAt: nostr.Timestamp(now.Unix())}
}

// GroupMetadataEvent builds a kind 9002 edit-metadata. private+closed make
// the group members-only for both reads and writes.
func GroupMetadataEvent(h, name, about string, private, closed bool, now time.Time) *nostr.Event {
	tags := nostr.Tags{{"h", h}, {"name", name}, {"about", about}}
	if private {
		tags = append(tags, nostr.Tag{"private"})
	}
	if closed {
		tags = append(tags, nostr.Tag{"closed"})
	}
	return &nostr.Event{Kind: nostr.KindSimpleGroupEditMetadata,
		Tags: tags, CreatedAt: nostr.Timestamp(now.Unix())}
}

// GroupPutUserEvent builds a kind 9000 adding a member to group h.
func GroupPutUserEvent(h, pubkey string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindSimpleGroupPutUser,
		Tags: nostr.Tags{{"h", h}, {"p", pubkey}}, CreatedAt: nostr.Timestamp(now.Unix())}
}

// ChatMessageEvent builds a kind 9 group chat message.
func ChatMessageEvent(h, content string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindSimpleGroupChatMessage,
		Tags: nostr.Tags{{"h", h}}, Content: content, CreatedAt: nostr.Timestamp(now.Unix())}
}

// GroupDeleteEventEvent builds a kind 9005 removing one event from group h
// (moderation — sender needs can_manage).
func GroupDeleteEventEvent(h, eventID string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindSimpleGroupDeleteEvent,
		Tags: nostr.Tags{{"h", h}, {"e", eventID}}, CreatedAt: nostr.Timestamp(now.Unix())}
}

// --- thread channel events (docs/nostr/channels.md) ---

const (
	// KindThreadRoot is the NIP-7D thread root.
	KindThreadRoot = 11
	// KindComment is the NIP-22 reply.
	KindComment = 1111
	// KindApproval is the NIP-72 approval, reused for thread approvals
	// (ADR 0010).
	KindApproval = 4550
	// KindLabel is the NIP-32 label (declines, lifecycle states).
	KindLabel = 1985
	// LabelNamespace is our NIP-32 namespace.
	LabelNamespace = "community.opencollective"
)

// ThreadRootEvent builds a kind 11 starting a thread in channel h. The
// visibility tag implements ADR 0012; extra carries template fields.
func ThreadRootEvent(h, title, content, visibility string, extra nostr.Tags, now time.Time) *nostr.Event {
	tags := nostr.Tags{{"h", h}, {"visibility", visibility}}
	if title != "" {
		tags = append(tags, nostr.Tag{"title", title})
	}
	tags = append(tags, extra...)
	return &nostr.Event{Kind: KindThreadRoot, Tags: tags, Content: content,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// ReplyEvent builds a kind 1111 reply to a thread root.
func ReplyEvent(h, rootID, content string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: KindComment,
		Tags:    nostr.Tags{{"h", h}, {"e", rootID, "", "root"}},
		Content: content, CreatedAt: nostr.Timestamp(now.Unix())}
}

// ReactionEvent builds a kind 7 emoji reaction (NIP-25).
func ReactionEvent(h, targetID, emoji string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindReaction,
		Tags:    nostr.Tags{{"h", h}, {"e", targetID}},
		Content: emoji, CreatedAt: nostr.Timestamp(now.Unix())}
}

// DeletionEvent builds a kind 5 (NIP-09) retracting one of the author's
// own events (reaction toggles).
func DeletionEvent(h, targetID string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindDeletion,
		Tags:      nostr.Tags{{"h", h}, {"e", targetID}},
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// ApprovalEvent builds a kind 4550 approving a thread root — the same
// signed-trail semantics as community publishing (ADR 0010). The approved
// event's JSON travels in the content, NIP-72 style. An empty h omits the
// group tag (publishing proposals are not group events).
func ApprovalEvent(h string, root *nostr.Event, communityPubkey, communitySlug string, now time.Time) *nostr.Event {
	raw, _ := json.Marshal(root)
	tags := nostr.Tags{}
	if h != "" {
		tags = append(tags, nostr.Tag{"h", h})
	}
	tags = append(tags,
		nostr.Tag{"a", fmt.Sprintf("%d:%s:%s", KindCommunityDefinition, communityPubkey, communitySlug)},
		nostr.Tag{"e", root.ID},
		nostr.Tag{"p", root.PubKey},
		nostr.Tag{"k", fmt.Sprint(root.Kind)},
	)
	return &nostr.Event{Kind: KindApproval, Tags: tags,
		Content: string(raw), CreatedAt: nostr.Timestamp(now.Unix())}
}

// DeclineEvent builds a kind 1985 'declined' label with a reason
// (CHAN-18). An empty h omits the group tag.
func DeclineEvent(h, rootID, reason string, now time.Time) *nostr.Event {
	tags := nostr.Tags{}
	if h != "" {
		tags = append(tags, nostr.Tag{"h", h})
	}
	tags = append(tags,
		nostr.Tag{"L", LabelNamespace},
		nostr.Tag{"l", "declined", LabelNamespace},
		nostr.Tag{"e", rootID},
	)
	return &nostr.Event{Kind: KindLabel, Tags: tags,
		Content: reason, CreatedAt: nostr.Timestamp(now.Unix())}
}

// --- publishing as the community (docs/nostr/publishing.md) ---

const (
	// KindAnnouncement is a community short note (NIP-01 kind 1).
	KindAnnouncement = 1
	// KindArticle is long-form content (NIP-23 kind 30023) — blog posts
	// and newsletters.
	KindArticle = 30023
)

// ContentKind maps a content type to its published Nostr kind.
func ContentKind(contentType string) int {
	if contentType == "announcement" {
		return KindAnnouncement
	}
	return KindArticle
}

// ProposalEvent builds a draft signed by the proposer (PUB-02): a kind 1
// (announcement) or kind 30023 (blog/newsletter) carrying the community
// a-tag and a ["proposal", <type>] marker. The d tag (for 30023) is unique
// so each submission is a distinct event — edits reset approvals (PUB-07).
func ProposalEvent(contentType, title, content, communityPubkey, communitySlug, uniqueD string, now time.Time) *nostr.Event {
	aTag := nostr.Tag{"a", fmt.Sprintf("%d:%s:%s", KindCommunityDefinition, communityPubkey, communitySlug)}
	tags := nostr.Tags{{"proposal", contentType}, aTag}
	kind := ContentKind(contentType)
	if kind == KindArticle {
		tags = append(tags, nostr.Tag{"d", uniqueD}, nostr.Tag{"title", title})
	}
	return &nostr.Event{Kind: kind, Tags: tags, Content: content,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// PublishedEvent builds the community-signed final artifact from an
// approved proposal: the same content, crediting the author and
// referencing the proposal (PUB-04/09). Newsletters carry a NIP-32
// self-label so only they are emailed (ADR 0011).
func PublishedEvent(contentType, title, content, slug, proposerPubkey, proposalID string, now time.Time) *nostr.Event {
	kind := ContentKind(contentType)
	tags := nostr.Tags{
		{"p", proposerPubkey, "", "author"},
		{"e", proposalID, "", "mention"},
	}
	if kind == KindArticle {
		tags = append(tags,
			nostr.Tag{"d", slug},
			nostr.Tag{"title", title},
			nostr.Tag{"published_at", fmt.Sprint(now.Unix())},
		)
	}
	if contentType == "newsletter" {
		tags = append(tags,
			nostr.Tag{"L", LabelNamespace},
			nostr.Tag{"l", "newsletter", LabelNamespace},
		)
	}
	return &nostr.Event{Kind: kind, Tags: tags, Content: content,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// --- calendar events (docs/nostr/channels.md § events, NIP-52) ---

const (
	// KindDateEvent is a NIP-52 all-day calendar event (kind 31922).
	KindDateEvent = 31922
	// KindTimeEvent is a NIP-52 time-based calendar event (kind 31923).
	KindTimeEvent = 31923
	// KindRSVP is a NIP-52 calendar RSVP (kind 31925).
	KindRSVP = 31925
)

// EventFields carries the event template's structured data.
type EventFields struct {
	Title    string
	Content  string
	AllDay   bool
	Start    int64  // unix seconds, or midnight UTC for all-day
	End      int64  // unix seconds
	Location string // place or online URL
	External string // optional external event-page URL
	Image    string // optional cover (Blossom URL)
	RRule    string // optional RFC 5545 subset; empty = does not repeat
	D        string // unique address tag
}

// EventRootEvent builds a NIP-52 calendar event as a channel thread root.
// All-day events are kind 31922 (date strings); timed events are kind
// 31923 (unix timestamps). Visibility and the channel h-tag layer the
// thread framework on top (ADR 0010/0012).
func EventRootEvent(h, visibility string, f EventFields, now time.Time) *nostr.Event {
	kind := KindTimeEvent
	tags := nostr.Tags{
		{"h", h},
		{"visibility", visibility},
		{"d", f.D},
		{"title", f.Title},
	}
	if f.AllDay {
		kind = KindDateEvent
		tags = append(tags,
			nostr.Tag{"start", dateStr(f.Start)},
			nostr.Tag{"end", dateStr(f.End)})
	} else {
		tags = append(tags,
			nostr.Tag{"start", fmt.Sprint(f.Start)},
			nostr.Tag{"end", fmt.Sprint(f.End)})
	}
	if f.Location != "" {
		tags = append(tags, nostr.Tag{"location", f.Location})
	}
	if f.External != "" {
		tags = append(tags, nostr.Tag{"r", f.External})
	}
	if f.Image != "" {
		tags = append(tags, nostr.Tag{"image", f.Image})
	}
	if f.RRule != "" {
		tags = append(tags, nostr.Tag{"rrule", f.RRule})
	}
	return &nostr.Event{Kind: kind, Tags: tags, Content: f.Content,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// RSVPEvent builds a kind 31925 RSVP. The d tag is the event id so a
// member's later answer replaces the earlier one (EVT-08).
func RSVPEvent(h, eventID, status string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: KindRSVP,
		Tags: nostr.Tags{
			{"h", h},
			{"d", eventID},
			{"e", eventID},
			{"status", status},
		},
		CreatedAt: nostr.Timestamp(now.Unix()),
	}
}

// CancelEvent builds a kind 1985 'cancelled' label on an event (EVT-06).
func CancelEvent(h, eventID string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: KindLabel,
		Tags: nostr.Tags{
			{"h", h},
			{"L", LabelNamespace},
			{"l", "cancelled", LabelNamespace},
			{"e", eventID},
		},
		CreatedAt: nostr.Timestamp(now.Unix()),
	}
}

func dateStr(unix int64) string {
	return time.Unix(unix, 0).UTC().Format("2006-01-02")
}

// --- community profile edits / linktree (docs/nostr/publishing.md § profile edits) ---

const (
	// KindAppData is NIP-78 application-specific data (kind 30078), used
	// as the profile-edit wrapper.
	KindAppData = 30078
)

// ProfileData is the editable community profile (kind 0 metadata plus a
// links array — a pragmatic extension other clients ignore).
type ProfileData struct {
	Name    string        `json:"name"`
	About   string        `json:"about"`
	Picture string        `json:"picture"`
	Links   []ProfileLink `json:"links,omitempty"`
}

// ProfileLink is one linktree entry.
type ProfileLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ProfileEditWrapper builds a kind 30078 wrapper signed by the proposer
// (PROF-01). A bare kind 0 would replace the proposer's own profile — the
// wrapper carries the proposed community profile as content instead, with
// a k:0 tag and a base hash for stale detection (PROF-08).
func ProfileEditWrapper(profileJSON, communityPubkey, communitySlug, uniqueD, baseHash string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: KindAppData,
		Tags: nostr.Tags{
			{"d", uniqueD},
			{"k", "0"},
			{"a", fmt.Sprintf("%d:%s:%s", KindCommunityDefinition, communityPubkey, communitySlug)},
			{"proposal", "profile"},
			{"base", baseHash},
		},
		Content:   profileJSON,
		CreatedAt: nostr.Timestamp(now.Unix()),
	}
}

// RawProfileEvent builds a community kind 0 from a metadata JSON string
// (PROF-03 — the bunker constructs this from the approved wrapper).
func RawProfileEvent(metadataJSON string, now time.Time) *nostr.Event {
	return &nostr.Event{Kind: nostr.KindProfileMetadata, Content: metadataJSON,
		CreatedAt: nostr.Timestamp(now.Unix())}
}

// IsNewsletter reports whether a published article carries the newsletter
// self-label.
func IsNewsletter(evt *nostr.Event) bool {
	for _, tag := range evt.Tags.GetAll([]string{"l", ""}) {
		if len(tag) > 1 && tag[1] == "newsletter" {
			return true
		}
	}
	return false
}

// CommunityDefinitionEvent builds the NIP-72 kind 34550.
func CommunityDefinitionEvent(slug, name, description, icon string, moderators []string, now time.Time) *nostr.Event {
	tags := nostr.Tags{
		{"d", slug},
		{"name", name},
		{"description", description},
	}
	if icon != "" {
		tags = append(tags, nostr.Tag{"image", icon})
	}
	for _, pk := range moderators {
		tags = append(tags, nostr.Tag{"p", pk, "", "moderator"})
	}
	return &nostr.Event{Kind: KindCommunityDefinition, Tags: tags,
		CreatedAt: nostr.Timestamp(now.Unix())}
}
