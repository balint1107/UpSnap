package pb

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type passkeyUser struct {
	record      *core.Record
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return []byte(u.record.Id) }
func (u *passkeyUser) WebAuthnName() string                       { return u.record.GetString("email") }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.WebAuthnName() }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

type ceremony struct {
	session       webauthn.SessionData
	user, purpose string
	expires       time.Time
}
type ceremonyStore struct {
	sync.Mutex
	pending map[string]ceremony
}

func (s *ceremonyStore) add(c ceremony) (string, error) {
	s.Lock()
	defer s.Unlock()
	for id, v := range s.pending {
		if time.Now().After(v.expires) {
			delete(s.pending, id)
		}
	}
	if len(s.pending) >= 1024 {
		return "", errors.New("busy")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	c.expires = time.Now().Add(5 * time.Minute)
	s.pending[id] = c
	return id, nil
}
func (s *ceremonyStore) take(id, purpose, user string) (webauthn.SessionData, error) {
	s.Lock()
	defer s.Unlock()
	c, ok := s.pending[id]
	delete(s.pending, id)
	if !ok || c.purpose != purpose || c.user != user || time.Now().After(c.expires) {
		return webauthn.SessionData{}, errors.New("invalid ceremony")
	}
	return c.session, nil
}
func registerPasskeys(se *core.ServeEvent, policy *loginNetworkPolicy) error {
	rp, origin := os.Getenv("UPSNAP_WEBAUTHN_RP_ID"), os.Getenv("UPSNAP_WEBAUTHN_ORIGIN")
	enabled := rp != "" && origin != ""
	se.Router.GET("/api/upsnap/passkeys/config", func(e *core.RequestEvent) error {
		e.Response.Header().Set("Cache-Control", "no-store")
		return e.JSON(200, map[string]bool{"enabled": enabled, "password_allowed": policy.passwordAllowed(e.Request)})
	})
	if !enabled {
		if rp != "" || origin != "" {
			return errors.New("configure both WebAuthn RP ID and origin")
		}
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() != rp || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("WebAuthn requires HTTPS origin matching RP ID")
	}
	w, err := webauthn.New(&webauthn.Config{RPID: rp, RPDisplayName: "Server monitor", RPOrigins: []string{origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}})
	if err != nil {
		return err
	}
	sessions := &ceremonyStore{pending: map[string]ceremony{}}
	rowsFor := func(id string) ([]*core.Record, error) {
		return se.App.FindRecordsByFilter("passkeys", "user = {:user} && rp_id = {:rp}", "", 0, 0, dbx.Params{"user": id, "rp": rp})
	}
	load := func(r *core.Record) (*passkeyUser, error) {
		rows, err := rowsFor(r.Id)
		if err != nil {
			return nil, err
		}
		user := &passkeyUser{record: r}
		for _, row := range rows {
			var c webauthn.Credential
			if err := row.UnmarshalJSONField("credential", &c); err != nil {
				return nil, err
			}
			user.credentials = append(user.credentials, c)
		}
		return user, nil
	}
	safe := func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
		return func(e *core.RequestEvent) error {
			e.Response.Header().Set("Cache-Control", "no-store")
			if e.Request.Method != "GET" && e.Request.Header.Get("Origin") != origin {
				return e.ForbiddenError("Invalid origin", nil)
			}
			e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 65536)
			return next(e)
		}
	}
	se.Router.GET("/api/upsnap/passkeys", safe(func(e *core.RequestEvent) error {
		rows, err := rowsFor(e.Auth.Id)
		if err != nil {
			return err
		}
		out := []map[string]string{}
		for _, r := range rows {
			out = append(out, map[string]string{"id": r.Id, "label": r.GetString("label"), "created": r.GetString("created"), "last_used": r.GetString("last_used")})
		}
		return e.JSON(200, out)
	})).Bind(apis.RequireAuth("users"))
	se.Router.DELETE("/api/upsnap/passkeys/{id}", safe(func(e *core.RequestEvent) error {
		r, err := se.App.FindRecordById("passkeys", e.Request.PathValue("id"))
		if err != nil || r.GetString("user") != e.Auth.Id || r.GetString("rp_id") != rp {
			return e.NotFoundError("Passkey not found", nil)
		}
		if err := se.App.Delete(r); err != nil {
			return err
		}
		return e.NoContent(204)
	})).Bind(apis.RequireAuth("users"))
	se.Router.POST("/api/upsnap/passkeys/register/begin", safe(func(e *core.RequestEvent) error {
		user, err := load(e.Auth)
		if err != nil {
			return err
		}
		if len(user.credentials) >= 10 {
			return e.BadRequestError("Maximum 10 passkeys", nil)
		}
		options, session, err := w.BeginRegistration(user, webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()))
		if err != nil {
			return e.BadRequestError("Cannot start enrollment", nil)
		}
		id, err := sessions.add(ceremony{session: *session, user: e.Auth.Id, purpose: "register"})
		if err != nil {
			return e.BadRequestError("Try again shortly", nil)
		}
		return e.JSON(200, map[string]any{"ceremony": id, "options": options})
	})).Bind(apis.RequireAuth("users"))
	se.Router.POST("/api/upsnap/passkeys/register/finish", safe(func(e *core.RequestEvent) error {
		session, err := sessions.take(e.Request.URL.Query().Get("ceremony"), "register", e.Auth.Id)
		if err != nil {
			return e.BadRequestError("Enrollment expired; try again", nil)
		}
		user, err := load(e.Auth)
		if err != nil {
			return err
		}
		if len(user.credentials) >= 10 {
			return e.BadRequestError("Maximum 10 passkeys", nil)
		}
		credential, err := w.FinishRegistration(user, session, e.Request)
		if err != nil {
			return e.BadRequestError("Passkey verification failed", nil)
		}
		collection, err := se.App.FindCollectionByNameOrId("passkeys")
		if err != nil {
			return err
		}
		r := core.NewRecord(collection)
		label := strings.TrimSpace(e.Request.URL.Query().Get("label"))
		if label == "" {
			label = "Passkey"
		}
		if len(label) > 80 {
			return e.BadRequestError("Label too long", nil)
		}
		r.Set("user", e.Auth.Id)
		r.Set("rp_id", rp)
		r.Set("label", label)
		r.Set("credential_id", base64.RawURLEncoding.EncodeToString(credential.ID))
		r.Set("credential", credential)
		if err := se.App.Save(r); err != nil {
			return e.BadRequestError("Cannot save passkey", nil)
		}
		return e.JSON(200, map[string]string{"id": r.Id})
	})).Bind(apis.RequireAuth("users"))
	se.Router.POST("/api/upsnap/passkeys/login/begin", safe(func(e *core.RequestEvent) error {
		options, session, err := w.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return e.BadRequestError("Cannot start login", nil)
		}
		id, err := sessions.add(ceremony{session: *session, purpose: "login"})
		if err != nil {
			return e.BadRequestError("Try again shortly", nil)
		}
		return e.JSON(200, map[string]any{"ceremony": id, "options": options})
	}))
	var loginMu sync.Mutex
	se.Router.POST("/api/upsnap/passkeys/login/finish", safe(func(e *core.RequestEvent) error {
		session, err := sessions.take(e.Request.URL.Query().Get("ceremony"), "login", "")
		if err != nil {
			return e.BadRequestError("Login expired; try again", nil)
		}
		if err := cachePasskeyRequest(e); err != nil {
			return e.BadRequestError("Invalid assertion body", nil)
		}
		loginMu.Lock()
		defer loginMu.Unlock()
		var stored *core.Record
		user, credential, err := w.FinishPasskeyLogin(func(rawID, handle []byte) (webauthn.User, error) {
			rows, err := se.App.FindRecordsByFilter("passkeys", "rp_id = {:rp} && credential_id = {:id}", "", 1, 0, dbx.Params{"rp": rp, "id": base64.RawURLEncoding.EncodeToString(rawID)})
			if err != nil || len(rows) != 1 {
				return nil, errors.New("unknown credential")
			}
			stored = rows[0]
			if !bytes.Equal(handle, []byte(stored.GetString("user"))) {
				return nil, errors.New("invalid handle")
			}
			r, err := se.App.FindRecordById("users", stored.GetString("user"))
			if err != nil {
				return nil, err
			}
			return load(r)
		}, session, e.Request)
		if err != nil || credential.Authenticator.CloneWarning {
			return e.UnauthorizedError("Passkey verification failed", nil)
		}
		stored.Set("credential", credential)
		stored.Set("last_used", time.Now().UTC())
		if err := se.App.Save(stored); err != nil {
			return err
		}
		return apis.RecordAuthResponse(e, user.(*passkeyUser).record, "passkey", nil)
	}))
	return nil
}

// WebAuthn consumes the assertion body. Preserve it for the verifier and cache
// request info for PocketBase auth-rule and MFA hooks before consuming it.
func cachePasskeyRequest(e *core.RequestEvent) error {
	body, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return err
	}
	e.Request.Body = io.NopCloser(bytes.NewReader(body))
	if _, err := e.RequestInfo(); err != nil {
		return err
	}
	e.Request.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}
