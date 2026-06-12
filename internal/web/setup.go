package web

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/opencollective/community/internal/auth"
	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/identity"
	"github.com/opencollective/community/internal/mail"
	"github.com/opencollective/community/internal/store"
)

// The setup wizard (docs/flows/setup.md): six steps, resumable, gone after
// completion. Step state is derived from what exists, never stored
// (SETUP-04).

const (
	setAdminIdentity = "admin_identity_id"
	setEmailProvider = "email_provider"
	setEmailAPIKey   = "email_api_key_enc" // encrypted with the DEK, hex
	setEmailFrom     = "email_from"
	setSetupComplete = "setup_complete"
	setName          = "name"
	setDescription   = "description"
	setIcon          = "icon"
	setCommunityID   = "community_identity_id"
)

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$|^(localhost|127\.0\.0\.1)$`)

// wizardStep computes the first incomplete step (2–6) for the community,
// or 7 when setup is complete.
func (a *App) wizardStep(c *store.Community) (int, error) {
	if _, err := c.Setting(setWrappedDEK); errors.Is(err, store.ErrNotFound) {
		return 2, nil
	} else if err != nil {
		return 0, err
	}
	if _, err := c.Setting(setAdminIdentity); errors.Is(err, store.ErrNotFound) {
		return 3, nil
	} else if err != nil {
		return 0, err
	}
	if _, err := c.Setting(setEmailFrom); errors.Is(err, store.ErrNotFound) {
		return 4, nil
	} else if err != nil {
		return 0, err
	}
	adminID, admin, err := a.adminIdentity(c)
	if err != nil {
		return 0, err
	}
	_ = adminID
	if admin.Email == "" || admin.Status != "active" {
		return 5, nil
	}
	if _, err := c.Setting(setSetupComplete); errors.Is(err, store.ErrNotFound) {
		return 6, nil
	} else if err != nil {
		return 0, err
	}
	return 7, nil
}

var stepPaths = map[int]string{
	2: "/setup/password",
	3: "/setup/admin",
	4: "/setup/email",
	5: "/setup/verify",
	6: "/setup/community",
}

// requireStep gates a wizard handler: redirects to the current step when
// the requested one doesn't match, and 404s once setup is done (SETUP-12).
func (a *App) requireStep(step int, h func(http.ResponseWriter, *http.Request, *store.Community)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := communityFrom(r)
		if c == nil {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}
		cur, err := a.wizardStep(c)
		if err != nil {
			a.internalError(w, err)
			return
		}
		switch {
		case cur >= 7:
			http.NotFound(w, r)
		case cur != step:
			http.Redirect(w, r, stepPaths[cur], http.StatusFound)
		default:
			h(w, r, c)
		}
	}
}

// --- step 1: domain ---

func (a *App) setupStep1(w http.ResponseWriter, r *http.Request) {
	if a.communityExists(w, r) {
		return
	}
	a.renderStep(w, 1, "setup_domain.html", map[string]any{"Domain": "", "Error": ""})
}

func (a *App) setupStep1Submit(w http.ResponseWriter, r *http.Request) {
	if a.communityExists(w, r) {
		return
	}
	domain := store.NormalizeHost(r.FormValue("domain"))
	if !domainRe.MatchString(domain) {
		a.renderStep(w, 1, "setup_domain.html", map[string]any{
			"Domain": domain,
			"Error":  "That does not look like a domain. Use something like community.example.org.",
		})
		return
	}
	if a.CheckDomain != nil {
		if err := a.CheckDomain(domain); err != nil {
			a.renderStep(w, 1, "setup_domain.html", map[string]any{
				"Domain": domain, "Error": err.Error(),
			})
			return
		}
	}
	if _, err := a.Store.CreateCommunity("main", domain); err != nil {
		a.internalError(w, err)
		return
	}
	// In production the TLS listener picks the new host up via its
	// registry-backed policy and this redirect lands on HTTPS (SETUP-03).
	target := "/setup/password"
	if !a.DevMode {
		target = "https://" + domain + "/setup/password"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *App) communityExists(w http.ResponseWriter, r *http.Request) bool {
	n, err := a.Store.CommunityCount()
	if err != nil {
		a.internalError(w, err)
		return true
	}
	if n > 0 {
		// Step 1 is over; send them to wherever the wizard stands.
		c := communityFrom(r)
		if c == nil {
			http.NotFound(w, r)
			return true
		}
		cur, err := a.wizardStep(c)
		if err != nil {
			a.internalError(w, err)
			return true
		}
		if cur >= 7 {
			http.NotFound(w, r)
		} else {
			http.Redirect(w, r, stepPaths[cur], http.StatusFound)
		}
		return true
	}
	return false
}

// --- step 2: master password ---

func (a *App) setupPassword(w http.ResponseWriter, r *http.Request, c *store.Community) {
	a.renderStep(w, 2, "setup_password.html", map[string]any{"Error": ""})
}

func (a *App) setupPasswordSubmit(w http.ResponseWriter, r *http.Request, c *store.Community) {
	pw, confirm := r.FormValue("password"), r.FormValue("confirm")
	strict := r.FormValue("strict") == "1"
	if len(pw) < 12 {
		a.renderStep(w, 2, "setup_password.html", map[string]any{
			"Error": "Use at least 12 characters — this password protects every key on the server.",
		})
		return
	}
	if pw != confirm {
		a.renderStep(w, 2, "setup_password.html", map[string]any{
			"Error": "The two passwords differ.",
		})
		return
	}
	if err := a.InitKeys(c, []byte(pw), strict); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/setup/admin", http.StatusSeeOther)
}

// --- step 3: admin account ---

func (a *App) setupAdmin(w http.ResponseWriter, r *http.Request, c *store.Community) {
	a.renderStep(w, 3, "setup_admin.html", map[string]any{"Username": "", "Error": "", "Host": c.Hostname})
}

func (a *App) setupAdminSubmit(w http.ResponseWriter, r *http.Request, c *store.Community) {
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	if err := identity.ValidateUsername(username); err != nil {
		a.renderStep(w, 3, "setup_admin.html", map[string]any{
			"Username": username, "Error": err.Error(), "Host": c.Hostname,
		})
		return
	}
	dek, ok := a.DEK(c)
	if !ok {
		a.internalError(w, fmt.Errorf("community locked during setup"))
		return
	}
	kp, err := identity.Generate()
	if err != nil {
		a.internalError(w, err)
		return
	}
	ident, err := a.createEncryptedIdentity(c, dek, kp, username, false)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.SetSetting(setAdminIdentity, fmt.Sprint(ident.ID)); err != nil {
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/setup/email", http.StatusSeeOther)
}

// encryptSecret produces the storage form of a private key: encrypted
// under the DEK, AAD-bound to the pubkey (KEY-06 — the pubkey is immutable
// and known before the row id is).
func encryptSecret(dek []byte, kp identity.KeyPair) ([]byte, error) {
	return crypto.Encrypt(dek, []byte(kp.SecretHex), []byte("nsec:"+kp.PublicHex))
}

func (a *App) createEncryptedIdentity(c *store.Community, dek []byte, kp identity.KeyPair, username string, isOrg bool) (*store.Identity, error) {
	enc, err := encryptSecret(dek, kp)
	if err != nil {
		return nil, err
	}
	return c.CreateIdentity(username, "", kp.PublicHex, enc, isOrg, "pending", a.Now())
}

// --- step 4: email provider ---

func (a *App) setupEmail(w http.ResponseWriter, r *http.Request, c *store.Community) {
	a.renderStep(w, 4, "setup_email.html", map[string]any{
		"From": "community@" + c.Hostname, "Error": "", "Records": nil,
	})
}

func (a *App) setupEmailSubmit(w http.ResponseWriter, r *http.Request, c *store.Community) {
	provider := r.FormValue("provider")
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	from := strings.ToLower(strings.TrimSpace(r.FormValue("from")))
	fail := func(msg string, records any) {
		a.renderStep(w, 4, "setup_email.html", map[string]any{
			"From": from, "Error": msg, "Records": records,
		})
	}
	if provider != "resend" || apiKey == "" || !strings.Contains(from, "@") {
		fail("Pick a provider, paste its API key, and choose a From address.", nil)
		return
	}
	m, err := a.MailerFactory(provider, apiKey, from)
	if err != nil {
		fail(err.Error(), nil)
		return
	}
	status, err := m.Verify(r.Context())
	if err != nil {
		fail("Could not reach the provider: "+err.Error(), nil)
		return
	}
	if !status.Verified {
		// SETUP-08: show the exact records, block the step.
		fail("This domain is not verified with the provider yet. Add these DNS records, then save again.", status.Records)
		return
	}
	dek, ok := a.DEK(c)
	if !ok {
		a.internalError(w, fmt.Errorf("community locked during setup"))
		return
	}
	keyEnc, err := crypto.Encrypt(dek, []byte(apiKey), []byte("setting:"+setEmailAPIKey))
	if err != nil {
		a.internalError(w, err)
		return
	}
	for k, v := range map[string]string{
		setEmailProvider: provider,
		setEmailAPIKey:   hex.EncodeToString(keyEnc),
		setEmailFrom:     from,
	} {
		if err := c.SetSetting(k, v); err != nil {
			a.internalError(w, err)
			return
		}
	}
	// SETUP-09: prove the pipeline with a real send.
	_ = m.Send(r.Context(), mailMessage([]string{from},
		"Your community server can send email",
		"This is the test email from your setup — everything works."))
	http.Redirect(w, r, "/setup/verify", http.StatusSeeOther)
}

// --- step 5: verify the admin's email ---

func (a *App) setupVerify(w http.ResponseWriter, r *http.Request, c *store.Community) {
	a.renderStep(w, 5, "setup_verify.html", map[string]any{
		"Email": "", "Sent": false, "Error": "",
	})
}

func (a *App) setupVerifySubmit(w http.ResponseWriter, r *http.Request, c *store.Community) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	code := strings.TrimSpace(r.FormValue("code"))

	if code == "" {
		if !strings.Contains(email, "@") {
			a.renderStep(w, 5, "setup_verify.html", map[string]any{
				"Email": email, "Sent": false, "Error": "Enter your email address.",
			})
			return
		}
		generated, err := auth.CreateCode(c, email, "verify", a.Argon2, a.Now())
		if errors.Is(err, auth.ErrRateLimited) {
			a.renderStep(w, 5, "setup_verify.html", map[string]any{
				"Email": email, "Sent": false, "Error": "Too many codes requested — wait a bit and try again.",
			})
			return
		}
		if err != nil {
			a.internalError(w, err)
			return
		}
		m, err := a.mailer(c)
		if err != nil {
			a.internalError(w, err)
			return
		}
		_ = m.Send(r.Context(), mailMessage([]string{email},
			"Your verification code",
			"Your code: "+generated+"\nIt expires in 10 minutes."))
		a.renderStep(w, 5, "setup_verify.html", map[string]any{
			"Email": email, "Sent": true, "Error": "",
		})
		return
	}

	if err := auth.VerifyCode(c, email, "verify", code, a.Argon2, a.Now()); err != nil {
		a.renderStep(w, 5, "setup_verify.html", map[string]any{
			"Email": email, "Sent": true, "Error": "That code is wrong or expired.",
		})
		return
	}
	_, admin, err := a.adminIdentity(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.BindEmail(admin.ID, email); err != nil {
		a.internalError(w, err)
		return
	}
	token, err := auth.CreateSession(c, admin.ID, a.Now())
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.setSessionCookie(w, token)
	http.Redirect(w, r, "/setup/community", http.StatusSeeOther)
}

// --- step 6: community profile ---

func (a *App) setupCommunity(w http.ResponseWriter, r *http.Request, c *store.Community) {
	a.renderStep(w, 6, "setup_community.html", map[string]any{"Error": ""})
}

func (a *App) setupCommunitySubmit(w http.ResponseWriter, r *http.Request, c *store.Community) {
	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	if name == "" {
		a.renderStep(w, 6, "setup_community.html", map[string]any{
			"Error": "Give your community a name.",
		})
		return
	}
	dek, ok := a.DEK(c)
	if !ok {
		a.internalError(w, fmt.Errorf("community locked during setup"))
		return
	}
	kp, err := identity.Generate()
	if err != nil {
		a.internalError(w, err)
		return
	}
	cid, err := a.createEncryptedIdentity(c, dek, kp, "community", false)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.CreateDefaultRoles(a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.CreateDefaultChannels(a.Now()); err != nil {
		a.internalError(w, err)
		return
	}
	_, admin, err := a.adminIdentity(c)
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := c.AssignRole(admin.ID, "steward"); err != nil {
		a.internalError(w, err)
		return
	}
	for k, v := range map[string]string{
		setName: name, setDescription: desc, setIcon: icon,
		setCommunityID: fmt.Sprint(cid.ID), setSetupComplete: "1",
	} {
		if err := c.SetSetting(k, v); err != nil {
			a.internalError(w, err)
			return
		}
	}
	if err := a.syncZooid(c); err != nil {
		a.Log.Error("sync zooid config", "err", err)
	}
	a.StartBunker(c)
	a.publishCommunityEvents(c)
	a.publishIdentityEvents(c, admin)
	a.createGeneralChannel(c)
	a.ensureChannelGroup(c, "proposals")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- helpers ---

func (a *App) adminIdentity(c *store.Community) (string, *store.Identity, error) {
	idStr, err := c.Setting(setAdminIdentity)
	if err != nil {
		return "", nil, err
	}
	var id int64
	fmt.Sscan(idStr, &id)
	ident, err := c.IdentityByID(id)
	return idStr, ident, err
}

func (a *App) renderStep(w http.ResponseWriter, step int, tmpl string, data map[string]any) {
	data["Title"] = "Set up your community"
	data["Step"] = step
	data["Steps"] = 6
	a.render(w, tmpl, data)
}

func mailMessage(to []string, subject, text string) mail.Message {
	return mail.Message{To: to, Subject: subject, Text: text}
}
