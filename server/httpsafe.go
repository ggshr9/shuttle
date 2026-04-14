package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ErrBlockedTarget is returned when an HTTP client refuses to dial or follow
// a redirect to an address that falls inside an SSRF-blocked CIDR range.
var ErrBlockedTarget = errors.New("safehttp: target address is blocked")

// lookupIPAddr is overridable in tests via httpsafe_testhook_test.go.
var lookupIPAddr = net.DefaultResolver.LookupIPAddr

// SafeDialContext returns a DialContext function suitable for http.Transport
// that refuses to connect to any address that resolves into the SSRF-blocked
// CIDR set (see server/ssrf.go). When allowPrivate is true, the check is
// bypassed entirely — intended for sandbox/testing environments only.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("safehttp: split host port: %w", err)
		}

		// Literal IP fast path.
		if ip := net.ParseIP(host); ip != nil {
			if !allowPrivate && isBlockedIP(ip) {
				return nil, fmt.Errorf("%w: %s", ErrBlockedTarget, ip.String())
			}
			return dialer.DialContext(ctx, network, addr)
		}

		// Hostname path: resolve, filter blocked IPs, try each allowed IP.
		ips, err := lookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("safehttp: resolve %q: %w", host, err)
		}
		var allowed []net.IPAddr
		for _, ipa := range ips {
			if allowPrivate || !isBlockedIP(ipa.IP) {
				allowed = append(allowed, ipa)
			}
		}
		if len(allowed) == 0 {
			return nil, fmt.Errorf("%w: all resolved addresses for %q are blocked", ErrBlockedTarget, host)
		}

		var lastErr error
		for _, ipa := range allowed {
			target := net.JoinHostPort(ipa.IP.String(), port)
			conn, dErr := dialer.DialContext(ctx, network, target)
			if dErr == nil {
				return conn, nil
			}
			lastErr = dErr
		}
		return nil, fmt.Errorf("safehttp: dial all allowed addresses failed: %w", lastErr)
	}
}

// SafeHTTPClientOptions configures a SafeHTTPClient.
type SafeHTTPClientOptions struct {
	Timeout              time.Duration
	AllowPrivateNetworks bool
	MaxRedirects         int
}

// NewSafeHTTPClient returns an *http.Client whose DialContext rejects
// connections to SSRF-blocked IPs, and whose CheckRedirect re-validates each
// redirect target host.
func NewSafeHTTPClient(opts SafeHTTPClientOptions) *http.Client {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 5
	}
	tr := &http.Transport{
		DialContext:           SafeDialContext(opts.AllowPrivateNetworks),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:       opts.Timeout,
		Transport:     tr,
		CheckRedirect: SafeCheckRedirect(opts.AllowPrivateNetworks, opts.MaxRedirects),
	}
}

// SafeCheckRedirect returns an http.Client.CheckRedirect func that rejects
// redirects whose target resolves into a blocked CIDR, and caps redirect
// depth.
//
// Policy note: redirects use a strict any-blocked policy — if any resolved IP
// of the redirect target is in a blocked range, the redirect is rejected.
// This differs from SafeDialContext, which uses a lenient all-blocked policy
// (dial proceeds if at least one resolved IP is public). Redirects are a
// higher-risk vector (attacker-controlled Location header), so strict is
// appropriate. SafeDialContext stays lenient to tolerate CDNs with mixed
// split-horizon DNS records.
func SafeCheckRedirect(allowPrivate bool, maxRedirects int) func(req *http.Request, via []*http.Request) error {
	if maxRedirects == 0 {
		maxRedirects = 5
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("safehttp: too many redirects (max %d)", maxRedirects)
		}
		if allowPrivate {
			return nil
		}
		host := req.URL.Hostname()
		if host == "" {
			return fmt.Errorf("safehttp: redirect target has no hostname")
		}
		if ip := net.ParseIP(host); ip != nil {
			if isBlockedIP(ip) {
				return fmt.Errorf("%w: redirect to %s", ErrBlockedTarget, ip.String())
			}
			return nil
		}
		ips, err := lookupIPAddr(req.Context(), host)
		if err != nil {
			return fmt.Errorf("%w: resolve redirect target %q: %v", ErrBlockedTarget, host, err)
		}
		for _, ipa := range ips {
			if isBlockedIP(ipa.IP) {
				return fmt.Errorf("%w: redirect to %s (%s)", ErrBlockedTarget, host, ipa.IP.String())
			}
		}
		return nil
	}
}
