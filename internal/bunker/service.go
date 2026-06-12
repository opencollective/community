package bunker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/nbd-wtf/go-nostr/nip44"
)

// Service listens on a relay for kind 24133 requests addressed to this
// community's bunker pubkeys and answers them
// (docs/architecture/bunker.md). In production the relay is the
// community's own zooid; in tests an in-process khatru.
type Service struct {
	Signer   *Signer
	RelayURL string
	Log      *slog.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	refresh chan struct{}
}

// Start launches the relay loop. Call Refresh after creating new bunker
// keys so the subscription picks them up.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.refresh = make(chan struct{}, 1)
	go s.run(ctx)
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Refresh re-subscribes (new bunker pubkeys exist).
func (s *Service) Refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresh != nil {
		select {
		case s.refresh <- struct{}{}:
		default:
		}
	}
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.session(ctx); err != nil && ctx.Err() == nil {
			s.Log.Error("bunker relay session", "relay", s.RelayURL, "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
}

// session connects and serves until the context ends. The subscription is
// kind-wide rather than per-bunker-pubkey so freshly created keys work
// without a resubscription gap; each request resolves the key set live.
// TODO(zooid milestone): tighten to #p filters once subscriptions can be
// updated without dropping in-flight ephemeral events.
func (s *Service) session(ctx context.Context) error {
	relay, err := nostr.RelayConnect(ctx, s.RelayURL)
	if err != nil {
		return err
	}
	defer relay.Close()

	// No Since: domain time is the injectable clock, but transport events
	// from external clients carry wall-clock timestamps — mixing the two
	// in a filter drops requests. The kind is ephemeral; replay is no
	// concern on the local relay.
	sub, err := relay.Subscribe(ctx, []nostr.Filter{{
		Kinds: []int{nostr.KindNostrConnect},
	}})
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.refresh:
			return nil // force reconnect
		case evt, ok := <-sub.Events:
			if !ok {
				return errors.New("subscription closed")
			}
			if evt == nil {
				continue
			}
			keys, err := s.Signer.C.BunkerIdentities()
			if err != nil {
				s.Log.Error("bunker identities", "err", err)
				continue
			}
			if resp := s.handle(ctx, keys, evt); resp != nil {
				if err := relay.Publish(ctx, *resp); err != nil {
					s.Log.Error("bunker publish response", "err", err)
				}
			}
		}
	}
}

type rpcRequest struct {
	ID     string   `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type rpcResponse struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// handle decrypts one request, applies session policy, and builds the
// encrypted response event.
func (s *Service) handle(_ context.Context, keys map[string]int64, evt *nostr.Event) *nostr.Event {
	bunkerPK := evt.Tags.GetFirst([]string{"p", ""})
	if bunkerPK == nil {
		return nil
	}
	identityID, ok := keys[(*bunkerPK)[1]]
	if !ok {
		return nil
	}
	bsk, bpk, err := s.Signer.bunkerSecret(identityID)
	if err != nil {
		s.Log.Error("bunker key unavailable", "err", err)
		return nil
	}

	plaintext, useNip04, err := decryptAny(bsk, evt.PubKey, evt.Content)
	if err != nil {
		return nil // not for us / garbage
	}
	var req rpcRequest
	if err := json.Unmarshal([]byte(plaintext), &req); err != nil {
		return nil
	}

	resp := s.dispatch(identityID, evt.PubKey, req)

	out, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	content, err := encryptAny(bsk, evt.PubKey, string(out), useNip04)
	if err != nil {
		return nil
	}
	reply := &nostr.Event{
		Kind: nostr.KindNostrConnect,
		// Wall clock on purpose: transport events interoperate with
		// external clients' filters; domain state uses the injectable
		// clock.
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"p", evt.PubKey}, {"e", evt.ID}},
		Content:   content,
		PubKey:    bpk,
	}
	if err := reply.Sign(bsk); err != nil {
		return nil
	}
	return reply
}

func (s *Service) dispatch(identityID int64, clientPK string, req rpcRequest) rpcResponse {
	c := s.Signer.C
	now := s.Signer.Now()
	fail := func(msg string) rpcResponse {
		return rpcResponse{ID: req.ID, Error: msg}
	}

	if req.Method == "connect" {
		secret := ""
		if len(req.Params) > 1 {
			secret = req.Params[1]
		}
		switch {
		case secret != "" && c.ConsumeBunkerSecret(identityID, secret, now):
			if err := c.CreateBunkerSession(identityID, clientPK, "", now); err != nil {
				return fail("session: " + err.Error())
			}
			return rpcResponse{ID: req.ID, Result: "ack"}
		case c.ActiveBunkerSession(identityID, clientPK, now):
			// A known client reconnecting: sessions survive restarts
			// (BUNKER-07); the stale secret in its config is ignored.
			return rpcResponse{ID: req.ID, Result: "ack"}
		default:
			return fail("invalid or expired secret")
		}
	}

	// Every other method requires a live session (BUNKER-02/05/06).
	if !c.ActiveBunkerSession(identityID, clientPK, now) {
		return fail("no session — connect first")
	}
	ident, err := c.IdentityByID(identityID)
	if err != nil {
		return fail("unknown identity")
	}

	switch req.Method {
	case "ping":
		return rpcResponse{ID: req.ID, Result: "pong"}

	case "get_public_key":
		return rpcResponse{ID: req.ID, Result: ident.Pubkey}

	case "sign_event":
		if len(req.Params) < 1 {
			return fail("missing event")
		}
		var evt nostr.Event
		if err := json.Unmarshal([]byte(req.Params[0]), &evt); err != nil {
			return fail("bad event: " + err.Error())
		}
		if err := s.Signer.SignAs(ident, &evt); err != nil {
			return fail("sign: " + err.Error())
		}
		out, err := json.Marshal(evt)
		if err != nil {
			return fail(err.Error())
		}
		return rpcResponse{ID: req.ID, Result: string(out)}

	case "nip04_encrypt", "nip04_decrypt", "nip44_encrypt", "nip44_decrypt":
		if len(req.Params) < 2 {
			return fail("missing params")
		}
		sk, err := s.Signer.identitySecret(ident)
		if err != nil {
			return fail("locked")
		}
		result, err := userCrypt(req.Method, sk, req.Params[0], req.Params[1])
		if err != nil {
			return fail(err.Error())
		}
		return rpcResponse{ID: req.ID, Result: result}

	default:
		return fail("unsupported method " + req.Method)
	}
}

// userCrypt runs the four encryption methods with the *identity's* key
// against a third party (BUNKER-08).
func userCrypt(method, sk, thirdParty, payload string) (string, error) {
	switch method {
	case "nip04_encrypt", "nip04_decrypt":
		shared, err := nip04.ComputeSharedSecret(thirdParty, sk)
		if err != nil {
			return "", err
		}
		if method == "nip04_encrypt" {
			return nip04.Encrypt(payload, shared)
		}
		return nip04.Decrypt(payload, shared)
	default:
		conv, err := nip44.GenerateConversationKey(thirdParty, sk)
		if err != nil {
			return "", err
		}
		if method == "nip44_encrypt" {
			return nip44.Encrypt(payload, conv)
		}
		return nip44.Decrypt(payload, conv)
	}
}

// decryptAny handles both transport encryptions: NIP-04 (legacy clients,
// recognizable by the ?iv= suffix) and NIP-44.
func decryptAny(bunkerSK, clientPK, content string) (plaintext string, useNip04 bool, err error) {
	if strings.Contains(content, "?iv=") {
		shared, err := nip04.ComputeSharedSecret(clientPK, bunkerSK)
		if err != nil {
			return "", true, err
		}
		pt, err := nip04.Decrypt(content, shared)
		return pt, true, err
	}
	conv, err := nip44.GenerateConversationKey(clientPK, bunkerSK)
	if err != nil {
		return "", false, err
	}
	pt, err := nip44.Decrypt(content, conv)
	return pt, false, err
}

func encryptAny(bunkerSK, clientPK, plaintext string, useNip04 bool) (string, error) {
	if useNip04 {
		shared, err := nip04.ComputeSharedSecret(clientPK, bunkerSK)
		if err != nil {
			return "", err
		}
		return nip04.Encrypt(plaintext, shared)
	}
	conv, err := nip44.GenerateConversationKey(clientPK, bunkerSK)
	if err != nil {
		return "", err
	}
	return nip44.Encrypt(plaintext, conv)
}

func ptr[T any](v T) *T { return &v }
