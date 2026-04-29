// Package dnsclass classifies resolver and dial errors into the small
// vocabulary of failure reasons emitted by the DNS / destination
// resolution metrics ("nxdomain", "timeout", "refused").
//
// It lives in a leaf package to break the import cycle that would
// otherwise form between router (which uses it from a metric hook) and
// server (which uses it when classifying net.Dial failures), since
// router transitively imports server via provider.
package dnsclass

import (
	"context"
	"errors"
	"net"
	"strings"
)

// Classify maps a resolver or dial error to one of the canonical reason
// labels exposed in shuttle_destination_resolve_failures_total:
//
//   - "nxdomain": the name has no record
//   - "timeout":  the upstream did not answer in time / context cancelled
//   - "refused":  any other resolver-side rejection (also the catch-all
//     for non-DNS dial errors such as "connection refused")
//
// Unknown errors fall through to "refused" rather than "timeout" so we
// don't inflate timeout rates on plain protocol failures.
func Classify(err error) string {
	if err == nil {
		return ""
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "nxdomain"
		}
		if dnsErr.IsTimeout {
			return "timeout"
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no such host"),
		strings.Contains(msg, "nxdomain"),
		strings.Contains(msg, "no addresses"):
		return "nxdomain"
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline"),
		strings.Contains(msg, "i/o timeout"):
		return "timeout"
	default:
		return "refused"
	}
}
