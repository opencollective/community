package web

import (
	"context"
	"fmt"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// newBunkerRelay builds the embedded NIP-46 transport relay served at
// /bunker. zooid (the data relay at /relay) requires NIP-42 membership to
// write, which external apps' ephemeral client keys can never satisfy —
// so signer traffic gets its own storage-free, auth-free, kind-24133-only
// relay. Events are ephemeral: khatru broadcasts them to live
// subscribers and nothing is persisted.
func newBunkerRelay() *khatru.Relay {
	relay := khatru.NewRelay()
	relay.Info.Name = "bunker transport"
	relay.Info.Description = "NIP-46 traffic only; nothing is stored"

	relay.RejectEvent = append(relay.RejectEvent,
		func(_ context.Context, evt *nostr.Event) (bool, string) {
			if evt.Kind != nostr.KindNostrConnect {
				return true, "restricted: this relay carries NIP-46 traffic only"
			}
			return false, ""
		})
	relay.RejectFilter = append(relay.RejectFilter,
		func(_ context.Context, f nostr.Filter) (bool, string) {
			for _, k := range f.Kinds {
				if k != nostr.KindNostrConnect {
					return true, "restricted: this relay carries NIP-46 traffic only"
				}
			}
			if len(f.Kinds) == 0 {
				return true, "restricted: subscribe to kind 24133 explicitly"
			}
			return false, ""
		})
	// No StoreEvent / QueryEvents: 24133 is ephemeral, broadcast-only.
	return relay
}

// bunkerRelayURL is the public websocket URL for NIP-46 traffic.
func (a *App) bunkerRelayURL(hostname string) string {
	scheme := "wss"
	if a.DevMode {
		scheme = "ws"
	}
	return fmt.Sprintf("%s://%s/bunker", scheme, hostname)
}
