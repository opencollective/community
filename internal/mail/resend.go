package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Resend implements Mailer over https://resend.com — plain HTTP calls, no
// SDK (docs/architecture/email.md).
type Resend struct {
	APIKey string
	From   string
	Base   string // overridable in tests
	HTTP   *http.Client
}

func NewResend(apiKey, from string) *Resend {
	return &Resend{
		APIKey: apiKey,
		From:   from,
		Base:   "https://api.resend.com",
		HTTP:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *Resend) Send(ctx context.Context, msg Message) error {
	payload := map[string]any{
		"from":    r.From,
		"to":      msg.To,
		"subject": msg.Subject,
		"text":    msg.Text,
	}
	if msg.HTML != "" {
		payload["html"] = msg.HTML
	}
	if msg.ListUnsubscribe != "" {
		payload["headers"] = map[string]string{"List-Unsubscribe": msg.ListUnsubscribe}
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := r.call(ctx, http.MethodPost, "/emails", payload, &out); err != nil {
		return fmt.Errorf("resend: send: %w", err)
	}
	return nil
}

func (r *Resend) Verify(ctx context.Context) (*DomainStatus, error) {
	fromDomain := r.From
	if i := strings.LastIndex(fromDomain, "@"); i >= 0 {
		fromDomain = fromDomain[i+1:]
	}
	var out struct {
		Data []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Records []struct {
				Record string `json:"record"`
				Type   string `json:"type"`
				Name   string `json:"name"`
				Value  string `json:"value"`
				Status string `json:"status"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := r.call(ctx, http.MethodGet, "/domains", nil, &out); err != nil {
		return nil, fmt.Errorf("resend: verify: %w", err)
	}
	for _, d := range out.Data {
		if !strings.EqualFold(d.Name, fromDomain) {
			continue
		}
		st := &DomainStatus{Verified: strings.EqualFold(d.Status, "verified")}
		if !st.Verified {
			for _, rec := range d.Records {
				st.Records = append(st.Records, DNSRecord{Type: rec.Type, Name: rec.Name, Value: rec.Value})
			}
		}
		return st, nil
	}
	return &DomainStatus{Verified: false, Records: []DNSRecord{
		{Type: "—", Name: fromDomain, Value: "domain not found in this Resend account — add it at resend.com/domains"},
	}}, nil
}

func (r *Resend) call(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
