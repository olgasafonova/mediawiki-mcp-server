package wiki

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateUploadDomain_FailsClosedWhenEnvUnset asserts the HG-3 fail-closed
// posture: with MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS unset, every URL is rejected.
func TestValidateUploadDomain_FailsClosedWhenEnvUnset(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "")

	urls := []string{
		"https://example.com/a.png",
		"https://attacker.example/poison.svg",
		"http://wiki.victim.tld/legit-looking.jpg",
	}
	for _, u := range urls {
		err := validateUploadDomain(u)
		if err == nil {
			t.Errorf("HG-3 regression: %q allowed with empty allowlist (must fail-closed)", u)
		}
		var ssrf *SSRFError
		if !errors.As(err, &ssrf) {
			t.Errorf("expected *SSRFError for %q, got %T: %v", u, err, err)
		}
	}
}

func TestValidateUploadDomain_FailsClosedWhenEnvWhitespace(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "   ,  ,   ")

	if err := validateUploadDomain("https://example.com/a.png"); err == nil {
		t.Error("HG-3 regression: whitespace-only allowlist treated as allow-all")
	}
}

func TestValidateUploadDomain_AcceptsExactHost(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "cdn.example.com,images.tieto.com")

	if err := validateUploadDomain("https://cdn.example.com/foo.png"); err != nil {
		t.Errorf("expected cdn.example.com to be allowed, got: %v", err)
	}
	if err := validateUploadDomain("https://images.tieto.com/bar.jpg"); err != nil {
		t.Errorf("expected images.tieto.com to be allowed, got: %v", err)
	}
}

func TestValidateUploadDomain_RejectsNonAllowlistedHost(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "cdn.example.com")

	err := validateUploadDomain("https://attacker.example/poison.svg")
	if err == nil {
		t.Fatal("expected attacker.example to be rejected")
	}
	if !strings.Contains(err.Error(), "attacker.example") {
		t.Errorf("expected error to name the rejected host, got: %v", err)
	}
}

func TestValidateUploadDomain_HandlesCase(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "CDN.Example.Com")

	if err := validateUploadDomain("https://cdn.example.com/foo.png"); err != nil {
		t.Errorf("case-insensitive match failed: %v", err)
	}
	if err := validateUploadDomain("https://CDN.EXAMPLE.COM/foo.png"); err != nil {
		t.Errorf("uppercase URL rejected: %v", err)
	}
}

func TestValidateUploadDomain_WildcardSubdomain(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "*.cdn.example.com")

	// Should match a subdomain of cdn.example.com.
	if err := validateUploadDomain("https://us.cdn.example.com/foo.png"); err != nil {
		t.Errorf("wildcard subdomain rejected: %v", err)
	}
	if err := validateUploadDomain("https://eu.frankfurt.cdn.example.com/foo.png"); err != nil {
		t.Errorf("nested wildcard subdomain rejected: %v", err)
	}

	// Wildcard "*.cdn.example.com" must NOT match cdn.example.com itself
	// (this is the conservative reading; if the operator wants the apex
	// they can list it separately). Critical because attacker may register
	// cdn.example.com.evil.com style hostnames otherwise.
	err := validateUploadDomain("https://cdn.example.com/foo.png")
	if err == nil {
		t.Error("wildcard pattern should not match its own apex")
	}

	// Must NOT match a different apex.
	err = validateUploadDomain("https://cdn.example.com.evil.com/foo.png")
	if err == nil {
		t.Error("HG-3 regression: wildcard accepted a host whose suffix happens to match")
	}
}

func TestValidateUploadDomain_MalformedURLRejected(t *testing.T) {
	t.Setenv(UploadAllowlistEnv, "example.com")

	for _, u := range []string{"://no-scheme", "http://", ""} {
		if err := validateUploadDomain(u); err == nil {
			t.Errorf("expected malformed URL %q to be rejected", u)
		}
	}
}
