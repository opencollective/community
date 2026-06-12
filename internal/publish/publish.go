// Package publish writes identities' events to the community's data relay
// (zooid): NIP-42 auth, relay membership joins with invite claims, and the
// profile / follow / community-definition events of docs/nostr.
package publish

import (
	"context"
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
	relay, err := nostr.RelayConnect(ctx, c.URL)
	if err != nil {
		return nil, fmt.Errorf("publish: connect %s: %w", c.URL, err)
	}
	return &session{relay: relay, ident: ident, c: c}, nil
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
