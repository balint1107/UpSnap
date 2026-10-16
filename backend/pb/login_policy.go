package pb

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Password restrictions are opt-in for generic deployments. When configured,
// only explicit networks may use password/OTP/OAuth recovery authentication.
// A proxy header is trusted solely from a configured socket peer, never by itself.
type loginNetworkPolicy struct{ private, proxies []netip.Prefix }

func parseLoginNetworks(value string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	if strings.TrimSpace(value) == "" {
		return result, nil
	}
	for _, value := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid authentication network: %w", err)
		}
		result = append(result, prefix)
	}
	return result, nil
}
func containsLoginIP(networks []netip.Prefix, ip netip.Addr) bool {
	for _, n := range networks {
		if n.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (p *loginNetworkPolicy) passwordAllowed(r *http.Request) bool {
	if len(p.private) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	if containsLoginIP(p.proxies, ip) {
		ip, err = netip.ParseAddr(r.Header.Get("X-Real-IP"))
		if err != nil {
			return false
		}
	}
	return containsLoginIP(p.private, ip)
}
func privateAuthenticationRoute(requestPath string) bool {
	cleaned := path.Clean(requestPath)
	if !strings.HasPrefix(cleaned, "/api/collections/") {
		return false
	}
	// Match the action for every collection name/id, including escaped or normalized paths.
	switch path.Base(cleaned) {
	case "auth-with-password", "auth-with-otp", "auth-with-oauth2", "request-otp", "request-password-reset", "confirm-password-reset":
		return true
	}
	return false
}
func registerLoginPolicy(se *core.ServeEvent) (*loginNetworkPolicy, error) {
	private, err := parseLoginNetworks(os.Getenv("UPSNAP_PASSWORD_ALLOWED_CIDRS"))
	if err != nil {
		return nil, err
	}
	proxies, err := parseLoginNetworks(os.Getenv("UPSNAP_TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return nil, err
	}
	if len(proxies) > 0 && len(private) == 0 {
		return nil, fmt.Errorf("trusted authentication proxies require allowed password networks")
	}
	p := &loginNetworkPolicy{private: private, proxies: proxies}
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		if privateAuthenticationRoute(e.Request.URL.Path) && !p.passwordAllowed(e.Request) {
			return e.ForbiddenError("Password and recovery sign-in require LAN or VPN access. Use a passkey here.", nil)
		}
		return e.Next()
	})
	return p, nil
}
