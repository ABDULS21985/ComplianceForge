// Package safehttp provides an outbound HTTP client that rejects common SSRF
// targets and resolves the exact address it subsequently dials.
package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrUnsafeURL = errors.New("unsafe outbound URL")

// Policy constrains outbound request destinations. HTTPS is always required.
// When AllowedHosts is empty, any publicly routable hostname is accepted.
type Policy struct {
	AllowedHosts []string
}

type resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// Client validates every request URL, disables environment proxies and
// redirects, then connects only to the vetted DNS result.
type Client struct {
	client *http.Client
	policy Policy
}

// NewClient constructs a hardened outbound client.
func NewClient(timeout time.Duration, policy Policy) *Client {
	return newClient(timeout, policy, net.DefaultResolver)
}

func newClient(timeout time.Duration, policy Policy, lookup resolver) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   minDuration(timeout, 10*time.Second),
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed destination", ErrUnsafeURL)
		}
		addresses, err := lookup.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve outbound host: %w", err)
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("resolve outbound host: no addresses")
		}
		for _, address := range addresses {
			if !isPublicIP(address.IP) {
				return nil, fmt.Errorf("%w: destination resolves to a non-public address", ErrUnsafeURL)
			}
		}
		var dialErrors []error
		for _, resolved := range addresses {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		return nil, fmt.Errorf("connect to outbound host: %w", errors.Join(dialErrors...))
	}
	return &Client{
		client: &http.Client{
			Timeout:       timeout,
			Transport:     transport,
			CheckRedirect: rejectRedirect,
		},
		policy: normalizePolicy(policy),
	}
}

// Do validates and performs one request. Redirects are rejected so a trusted
// public endpoint cannot bounce a request into an internal network.
func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("%w: request URL is required", ErrUnsafeURL)
	}
	if err := ValidateURL(request.URL, c.policy); err != nil {
		return nil, err
	}
	return c.client.Do(request)
}

// ValidateURL rejects credentials, fragments, plaintext HTTP, local names and
// literal non-public IP addresses before any network operation is attempted.
func ValidateURL(destination *url.URL, policy Policy) error {
	if destination == nil || !strings.EqualFold(destination.Scheme, "https") || destination.Host == "" {
		return fmt.Errorf("%w: destination must be an absolute HTTPS URL", ErrUnsafeURL)
	}
	if destination.User != nil || destination.Fragment != "" {
		return fmt.Errorf("%w: credentials and fragments are not allowed", ErrUnsafeURL)
	}
	hostname := strings.ToLower(strings.TrimSuffix(destination.Hostname(), "."))
	if hostname == "" || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
		return fmt.Errorf("%w: local destinations are not allowed", ErrUnsafeURL)
	}
	if port := destination.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return fmt.Errorf("%w: invalid destination port", ErrUnsafeURL)
		}
	}
	normalizedPolicy := normalizePolicy(policy)
	if len(normalizedPolicy.AllowedHosts) > 0 && !contains(normalizedPolicy.AllowedHosts, hostname) {
		return fmt.Errorf("%w: destination host is not allowed", ErrUnsafeURL)
	}
	if parsedIP := net.ParseIP(hostname); parsedIP != nil && !isPublicIP(parsedIP) {
		return fmt.Errorf("%w: non-public IP destinations are not allowed", ErrUnsafeURL)
	}
	return nil
}

func normalizePolicy(policy Policy) Policy {
	hosts := make([]string, 0, len(policy.AllowedHosts))
	for _, host := range policy.AllowedHosts {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if host != "" && !contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	policy.AllowedHosts = hosts
	return policy
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	blockedCIDRs := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"2001:db8::/32", "2001:2::/48", "2001:10::/28",
	}
	for _, rawCIDR := range blockedCIDRs {
		_, network, _ := net.ParseCIDR(rawCIDR)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return fmt.Errorf("outbound redirects are disabled")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
