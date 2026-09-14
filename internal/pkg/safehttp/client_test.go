package safehttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateURLRejectsUnsafeDestinations(t *testing.T) {
	tests := []string{
		"http://example.com/hook",
		"https://user:secret@example.com/hook",
		"https://localhost/hook",
		"https://api.internal/hook",
		"https://127.0.0.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://10.0.0.1/hook",
		"https://[::1]/hook",
		"https://example.com/hook#fragment",
	}
	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			destination, err := url.Parse(rawURL)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateURL(destination, Policy{}); !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("ValidateURL(%q) error = %v, want ErrUnsafeURL", rawURL, err)
			}
		})
	}
}

func TestValidateURLHonorsExactHostAllowlist(t *testing.T) {
	allowed, _ := url.Parse("https://hooks.slack.com/services/a/b/c")
	if err := ValidateURL(allowed, Policy{AllowedHosts: []string{"HOOKS.SLACK.COM."}}); err != nil {
		t.Fatalf("ValidateURL() error = %v", err)
	}
	notAllowed, _ := url.Parse("https://hooks.slack.com.attacker.example/services/a")
	if err := ValidateURL(notAllowed, Policy{AllowedHosts: []string{"hooks.slack.com"}}); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("ValidateURL() error = %v, want ErrUnsafeURL", err)
	}
}

func TestClientBlocksPrivateDNSResolution(t *testing.T) {
	client := newClient(time.Second, Policy{}, staticResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://webhook.example/hook", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(request)
	if !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("Do() error = %v, want ErrUnsafeURL", err)
	}
}

type staticResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, r.err
}
