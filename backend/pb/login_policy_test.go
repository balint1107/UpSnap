package pb

import (
	"net/http/httptest"
	"testing"
)

func TestPasswordNetworkPolicy(t *testing.T) {
	private, _ := parseLoginNetworks("192.168.50.0/24,10.8.0.0/24,127.0.0.1/32,::1/128,172.17.0.1/32")
	proxies, _ := parseLoginNetworks("172.18.0.3/32")
	p := &loginNetworkPolicy{private: private, proxies: proxies}
	for _, tc := range []struct {
		name, peer, realIP, forwarded string
		allowed                       bool
	}{
		{"LAN", "192.168.50.42:1000", "", "", true},
		{"VPN", "10.8.0.9:1000", "", "", true},
		{"local maintenance", "172.17.0.1:1000", "", "", true},
		{"IPv6 local", "[::1]:1000", "", "", true},
		{"mapped LAN", "[::ffff:192.168.50.42]:1000", "", "", true},
		{"public", "203.0.113.5:1000", "", "", false},
		{"spoofed direct headers", "203.0.113.5:1000", "192.168.50.42", "10.8.0.9", false},
		{"other container spoof", "172.18.0.8:1000", "192.168.50.42", "", false},
		{"proxied LAN", "172.18.0.3:1000", "192.168.50.42", "203.0.113.5", true},
		{"proxied VPN", "172.18.0.3:1000", "10.8.0.9", "", true},
		{"proxied public with spoofed XFF", "172.18.0.3:1000", "203.0.113.5", "192.168.50.42", false},
		{"missing proxy header", "172.18.0.3:1000", "", "192.168.50.42", false},
		{"multi address header", "172.18.0.3:1000", "192.168.50.42,203.0.113.5", "", false},
		{"malformed peer", "invalid", "192.168.50.42", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://example.test/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Real-IP", tc.realIP)
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			if got := p.passwordAllowed(r); got != tc.allowed {
				t.Fatalf("allowed=%v; expected %v", got, tc.allowed)
			}
		})
	}
}
func TestPrivateAuthenticationRoutes(t *testing.T) {
	for _, p := range []string{"/api/collections/users/auth-with-password", "/api/collections/_superusers/auth-with-password", "/api/collections/randomcollectionid/auth-with-password/", "/api/collections//users/auth-with-password", "/api/collections/users/request-password-reset", "/api/collections/users/confirm-password-reset", "/api/collections/users/auth-with-otp", "/api/collections/users/request-otp", "/api/collections/users/auth-with-oauth2"} {
		if !privateAuthenticationRoute(p) {
			t.Errorf("unrestricted authentication path: %s", p)
		}
	}
	for _, p := range []string{"/api/upsnap/passkeys/login/begin", "/api/upsnap/passkeys/login/finish", "/api/collections/users/auth-refresh", "/api/collections/users/records", "/api/collections/users/auth-methods"} {
		if privateAuthenticationRoute(p) {
			t.Errorf("incorrectly restricted %s", p)
		}
	}
}
func TestLoginPolicyConfiguration(t *testing.T) {
	if _, err := parseLoginNetworks("192.168.50.0/24,invalid"); err == nil {
		t.Fatal("malformed network accepted")
	}
	r := httptest.NewRequest("POST", "https://example.test/", nil)
	r.RemoteAddr = "203.0.113.5:1000"
	if !(&loginNetworkPolicy{}).passwordAllowed(r) {
		t.Fatal("unconfigured upstream deployment unexpectedly restricted")
	}
}
