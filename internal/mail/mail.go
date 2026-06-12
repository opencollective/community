// Package mail is the pluggable email layer of
// docs/architecture/email.md: one interface, one file per provider.
package mail

import (
	"context"
	"fmt"
)

// Message is one outgoing email. Text is always present; HTML is set for
// newsletters. ListUnsubscribe carries the RFC 8058 header value when set.
type Message struct {
	To              []string
	Subject         string
	Text            string
	HTML            string
	ListUnsubscribe string
}

// DNSRecord is one record the operator must add for domain verification.
type DNSRecord struct {
	Type  string
	Name  string
	Value string
}

// DomainStatus reports whether the From domain may send, with actionable
// details when it may not (SETUP-08).
type DomainStatus struct {
	Verified bool
	Records  []DNSRecord
}

// Mailer sends email and verifies the sending domain.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
	Verify(ctx context.Context) (*DomainStatus, error)
}

// Factory builds a Mailer from stored provider config. The web layer keeps
// one so tests can inject a fake (docs/testing/environment.md).
type Factory func(provider, apiKey, from string) (Mailer, error)

// New is the production factory.
func New(provider, apiKey, from string) (Mailer, error) {
	switch provider {
	case "resend":
		return NewResend(apiKey, from), nil
	default:
		return nil, fmt.Errorf("mail: unknown provider %q", provider)
	}
}
