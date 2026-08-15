package security

import (
	"fmt"
	"net"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

// ValidateListenAddr implements the proxy bind policy from
// docs/09-security.md section 5 / docs/04-interfaces.md section 13.
// Reserved for the proxy (v3), included now so the security package's
// public contract is stable per ADR-010 / docs/02-packages.md.
func ValidateListenAddr(addr string, allowRemote bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Allow bare hosts without a port (rare, but don't crash on it).
		host = addr
	}
	host = strings.TrimSpace(host)
	if isLoopbackHost(host) {
		return nil
	}
	if allowRemote {
		return nil
	}
	return domain.NewSecurityError(fmt.Sprintf(
		"refusing to bind proxy on non-loopback address %q without --allow-remote", addr))
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false // "" means all interfaces in Go — never loopback-safe
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
