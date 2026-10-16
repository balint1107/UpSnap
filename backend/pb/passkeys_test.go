package pb

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCeremonyIsolationAndReplay(t *testing.T) {
	for _, tc := range []struct{ name, purpose, user string }{{"valid", "register", "user1"}, {"wrong account", "register", "user2"}, {"wrong purpose", "login", "user1"}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &ceremonyStore{pending: map[string]ceremony{}}
			id, err := s.add(ceremony{purpose: "register", user: "user1"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.take(id, tc.purpose, tc.user)
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("unexpected validation: %v", err)
			}
			if _, err = s.take(id, "register", "user1"); err == nil {
				t.Fatal("replay accepted")
			}
		})
	}
}
func TestCeremonyExpiryAndCapacity(t *testing.T) {
	s := &ceremonyStore{pending: map[string]ceremony{"expired": {expires: time.Now().Add(-time.Second)}}}
	if _, err := s.take("expired", "", ""); err == nil {
		t.Fatal("expired ceremony accepted")
	}
	for i := 0; i < 1024; i++ {
		if _, err := s.add(ceremony{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.add(ceremony{}); err == nil {
		t.Fatal("unbounded pending ceremonies")
	}
	for id := range s.pending {
		v := s.pending[id]
		v.expires = time.Now().Add(-time.Second)
		s.pending[id] = v
		break
	}
	if _, err := s.add(ceremony{}); err != nil {
		t.Fatal("expired entries not pruned")
	}
}

func TestPasskeyBodySurvivesVerifierAndAuthHooks(t *testing.T) {
	const body = `{"id":"public-credential-id","response":{"signature":"public-signature"}}`
	request := httptest.NewRequest(http.MethodPost, "https://example.test/api/upsnap/passkeys/login/finish", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	e := &core.RequestEvent{Event: router.Event{Request: request, Response: httptest.NewRecorder()}}
	if err := cachePasskeyRequest(e); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(e.Request.Body)
	if err != nil || string(raw) != body {
		t.Fatal("assertion body changed")
	}
	info, err := e.RequestInfo()
	if err != nil || info.Body["id"] != "public-credential-id" {
		t.Fatalf("auth hooks lost request body: %v", err)
	}
}
func TestPasskeyBodyLimitsAndInvalidJSON(t *testing.T) {
	for _, body := range []string{`{"id":`, `{"id":"` + strings.Repeat("x", 65536) + `"}`} {
		request := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		request.Body = http.MaxBytesReader(response, request.Body, 65536)
		e := &core.RequestEvent{Event: router.Event{Request: request, Response: response}}
		if err := cachePasskeyRequest(e); err == nil {
			t.Fatal("invalid or oversized assertion accepted")
		}
	}
}
