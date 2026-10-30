package pb

import (
	"github.com/pocketbase/pocketbase/core"
	"os"
	"path/filepath"
	"testing"
)

// TestTrustedSSOUser verifies that only a secret-verified configured identity maps to an existing account.
func TestTrustedSSOUser(t *testing.T) {
	config := ssoSettings{Secret: "01234567890123456789012345678901", Users: map[string]string{"balint": "existing-record"}}
	for _, test := range []struct{ secret, user, want string }{
		{config.Secret, "balint", "existing-record"}, {"", "balint", ""}, {"wrong", "balint", ""},
		{config.Secret, "unknown", ""}, {config.Secret, "balint\n", ""},
	} {
		if got := trustedSSOUser(config, test.secret, test.user); got != test.want {
			t.Fatalf("unexpected identity mapping for %q", test.user)
		}
	}
	config.Secret = ""
	if trustedSSOUser(config, "", "balint") != "" {
		t.Fatal("empty secret accepted")
	}
}

// TestServiceAdministrator separates explicit routing grants from ordinary authenticated users and device grants.
func TestServiceAdministrator(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sso.json")
	if err := os.WriteFile(file, []byte(`{"admin_ids":["existing-admin"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPSNAP_SSO_FILE", file)
	event := &core.RequestEvent{}
	if serviceAdministrator(event) {
		t.Fatal("anonymous routing access")
	}
	event.Auth = core.NewRecord(core.NewAuthCollection("users"))
	event.Auth.Id = "ordinary-user"
	if serviceAdministrator(event) {
		t.Fatal("ordinary user routing access")
	}
	event.Auth.Id = "existing-admin"
	if !serviceAdministrator(event) {
		t.Fatal("explicit routing administrator denied")
	}
	event.Auth = core.NewRecord(core.NewAuthCollection("_superusers"))
	event.Auth.Id = "superuser"
	if !serviceAdministrator(event) {
		t.Fatal("superuser denied")
	}
}
