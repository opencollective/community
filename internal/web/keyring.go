package web

import (
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/opencollective/community/internal/crypto"
	"github.com/opencollective/community/internal/store"
)

// keyring holds each community's DEK in memory
// (docs/architecture/key-management.md). On open it attempts auto-unlock
// via the machine key (KEY-03); in strict mode the community stays locked
// until /unlock (KEY-04).
type keyring struct {
	mu   sync.Mutex
	deks map[string][]byte // community slug -> DEK
}

func newKeyring() *keyring {
	return &keyring{deks: map[string][]byte{}}
}

// settings keys for key material
const (
	setWrappedDEK        = "wrapped_dek"         // password-wrapped, hex
	setMachineWrappedDEK = "machine_wrapped_dek" // machine-key-wrapped, hex
	setStrictMode        = "strict_mode"         // "1" when enabled
)

func machineKeyPath(c *store.Community) string {
	return filepath.Join(c.Dir, "secrets", "machine.key")
}

// InitKeys runs at wizard step 2: generates the DEK, wraps it with the
// password (and the machine key unless strict), and caches it (SETUP-06).
func (a *App) InitKeys(c *store.Community, password []byte, strict bool) error {
	dek, err := crypto.NewDEK()
	if err != nil {
		return err
	}
	wrapped, err := crypto.WrapDEK(dek, password, a.Argon2)
	if err != nil {
		return err
	}
	if err := c.SetSetting(setWrappedDEK, hex.EncodeToString(wrapped)); err != nil {
		return err
	}
	if strict {
		if err := c.SetSetting(setStrictMode, "1"); err != nil {
			return err
		}
	} else {
		mk, err := crypto.NewMachineKey()
		if err != nil {
			return err
		}
		if err := os.WriteFile(machineKeyPath(c), mk, 0o600); err != nil {
			return err
		}
		mWrapped, err := crypto.WrapDEKWithKey(dek, mk)
		if err != nil {
			return err
		}
		if err := c.SetSetting(setMachineWrappedDEK, hex.EncodeToString(mWrapped)); err != nil {
			return err
		}
	}
	a.keys.mu.Lock()
	a.keys.deks[c.Slug] = dek
	a.keys.mu.Unlock()
	return nil
}

// DEK returns the community's data encryption key, attempting auto-unlock
// on first use after a restart. ok is false when the community is locked
// (strict mode, no password entered yet).
func (a *App) DEK(c *store.Community) ([]byte, bool) {
	a.keys.mu.Lock()
	defer a.keys.mu.Unlock()
	if dek, ok := a.keys.deks[c.Slug]; ok {
		return dek, true
	}
	mWrappedHex, err := c.Setting(setMachineWrappedDEK)
	if err != nil {
		return nil, false
	}
	mk, err := os.ReadFile(machineKeyPath(c))
	if err != nil {
		return nil, false
	}
	mWrapped, err := hex.DecodeString(mWrappedHex)
	if err != nil {
		return nil, false
	}
	dek, err := crypto.UnwrapDEKWithKey(mWrapped, mk)
	if err != nil {
		a.Log.Error("auto-unlock failed", "community", c.Slug, "err", err)
		return nil, false
	}
	a.keys.deks[c.Slug] = dek
	return dek, true
}

// Unlock opens a locked community with the master password.
func (a *App) Unlock(c *store.Community, password []byte) error {
	wrappedHex, err := c.Setting(setWrappedDEK)
	if err != nil {
		return err
	}
	wrapped, err := hex.DecodeString(wrappedHex)
	if err != nil {
		return err
	}
	dek, err := crypto.UnwrapDEK(wrapped, password)
	if err != nil {
		return err
	}
	a.keys.mu.Lock()
	a.keys.deks[c.Slug] = dek
	a.keys.mu.Unlock()
	return nil
}

func (a *App) unlockPage(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.DEK(c); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	a.render(w, "unlock.html", map[string]any{"Title": "Unlock", "Error": ""})
}

func (a *App) unlockSubmit(w http.ResponseWriter, r *http.Request) {
	c := communityFrom(r)
	if c == nil {
		http.NotFound(w, r)
		return
	}
	if err := a.Unlock(c, []byte(r.FormValue("password"))); err != nil {
		if errors.Is(err, crypto.ErrWrongKey) {
			a.render(w, "unlock.html", map[string]any{
				"Title": "Unlock", "Error": "Wrong master password.",
			})
			return
		}
		a.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
