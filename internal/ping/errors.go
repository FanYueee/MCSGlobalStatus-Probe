package ping

import (
	"net"
	"regexp"
	"strings"
)

// Regex patterns to match sensitive information in error messages
var (
	// Match IP:port patterns (both IPv4 and IPv6)
	ipPortRegex = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d+`)
	ipv6PortRegex = regexp.MustCompile(`\[[^\]]+\]:\d+`)
	// Match "on X.X.X.X:53" DNS resolver info
	dnsResolverRegex = regexp.MustCompile(` on \d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d+`)
)

// sanitizeError converts raw error messages to safe, generic messages
// that don't leak internal network information
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Check for specific error types and return safe messages
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return "Connection timed out"
		}
	}

	// DNS lookup errors
	if strings.Contains(errStr, "no such host") {
		return "DNS lookup failed: no such host"
	}
	if strings.Contains(errStr, "lookup") && strings.Contains(errStr, "no such host") {
		return "DNS lookup failed: no such host"
	}

	// Connection refused
	if strings.Contains(errStr, "connection refused") {
		return "Connection refused"
	}

	// Connection reset
	if strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "ECONNRESET") {
		return "Connection reset by peer"
	}

	// Network unreachable
	if strings.Contains(errStr, "network is unreachable") || strings.Contains(errStr, "ENETUNREACH") {
		return "Network unreachable"
	}

	// Host unreachable
	if strings.Contains(errStr, "host is unreachable") || strings.Contains(errStr, "EHOSTUNREACH") {
		return "Host unreachable"
	}

	// Timeout variations
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
		return "Connection timed out"
	}

	// EOF (connection closed)
	if strings.Contains(errStr, "EOF") || errStr == "EOF" {
		return "Connection closed unexpectedly"
	}

	// I/O timeout
	if strings.Contains(errStr, "i/o timeout") {
		return "Connection timed out"
	}

	// For any other errors, strip out sensitive information
	sanitized := errStr
	sanitized = dnsResolverRegex.ReplaceAllString(sanitized, "")
	sanitized = ipPortRegex.ReplaceAllString(sanitized, "[redacted]")
	sanitized = ipv6PortRegex.ReplaceAllString(sanitized, "[redacted]")

	return sanitized
}
