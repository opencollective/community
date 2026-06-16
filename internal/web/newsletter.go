package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nbd-wtf/go-nostr"

	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/internal/markdown"
	"github.com/opencollective/community/internal/store"
)

// Newsletter delivery (docs/architecture/email.md): only published
// newsletters are emailed (MAIL-04 — announcements and blog are not), at
// most once (MAIL-05), with text + sanitized HTML parts (MAIL-02) and a
// one-click unsubscribe (MAIL-03).

func (a *App) sendNewsletter(c *store.Community, evt *nostr.Event, title string) {
	claimed, err := c.ClaimNewsletterSend(evt.ID, a.Now())
	if err != nil {
		a.Log.Error("newsletter claim", "err", err)
		return
	}
	if !claimed {
		return // already sent (MAIL-05)
	}
	recipients, err := c.NewsletterRecipients()
	if err != nil {
		a.Log.Error("newsletter recipients", "err", err)
		return
	}
	m, err := a.mailer(c)
	if err != nil {
		a.Log.Error("newsletter mailer", "err", err)
		return
	}

	slug := ""
	if d := evt.Tags.GetFirst([]string{"d", ""}); d != nil && len(*d) > 1 {
		slug = (*d)[1]
	}
	base := a.publicURL(c, "http", "")
	if !a.DevMode {
		base = "https://" + c.Hostname
	}
	postURL := base + "/posts/" + slug

	htmlBody := markdown.HTML(evt.Content)
	textBody := markdown.Text(evt.Content)

	ctx, cancel := context.WithTimeout(a.baseCtx, 60*publishTimeout/10)
	defer cancel()

	sent := 0
	for _, to := range recipients {
		unsub := fmt.Sprintf("%s/unsubscribe?email=%s", base, to)
		msg := mail.Message{
			To:      []string{to},
			Subject: title,
			Text:    textBody + "\n\n—\nRead online: " + postURL + "\nUnsubscribe: " + unsub,
			HTML: htmlBody +
				`<hr><p><a href="` + postURL + `">Read online</a> · ` +
				`<a href="` + unsub + `">Unsubscribe</a></p>`,
			ListUnsubscribe: "<" + unsub + ">",
		}
		if err := m.Send(ctx, msg); err != nil {
			a.Log.Error("newsletter send", "to", to, "err", err)
			continue
		}
		sent++
	}
	if err := c.RecordNewsletterSent(evt.ID, sent); err != nil {
		a.Log.Error("newsletter record", "err", err)
	}
}

// unsubscribe clears a recipient's newsletter opt-in without login
// (FOLLOW-06, MAIL-03).
func (a *App) unsubscribe(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	email := r.URL.Query().Get("email")
	if ident, err := c.IdentityByEmail(email); err == nil {
		_ = c.UpdateProfile(ident.ID, ident.Name, false)
	}
	// Same response whether or not the address existed.
	a.render(w, "unsubscribed.html", map[string]any{"Title": "Unsubscribed"})
}
