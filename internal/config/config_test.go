package config

import (
	"strings"
	"testing"
)

func TestLoadSample(t *testing.T) {
	// The upstream sample must parse as-is: existing s3tests.conf files
	// working unchanged is the whole point of this package.
	cfg, err := Load("../../s3tests.conf.SAMPLE")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Endpoint != "http://localhost:8000" {
		t.Errorf("Endpoint = %q, want http://localhost:8000", cfg.Endpoint)
	}
	if cfg.IsSecure {
		t.Error("IsSecure = true, want false")
	}
	if cfg.Main.AccessKey != "0555b35654ad1656d804" {
		t.Errorf("Main.AccessKey = %q", cfg.Main.AccessKey)
	}
	if cfg.Main.DisplayName != "M. Tester" {
		t.Errorf("Main.DisplayName = %q, want %q", cfg.Main.DisplayName, "M. Tester")
	}
	if cfg.Alt.AccessKey != "NOPQRSTUVWXYZABCDEFG" {
		t.Errorf("Alt.AccessKey = %q", cfg.Alt.AccessKey)
	}
	if cfg.TenantName != "testx" {
		t.Errorf("TenantName = %q, want testx", cfg.TenantName)
	}
	if cfg.APIName != "default" {
		t.Errorf("APIName = %q, want default", cfg.APIName)
	}
	// "yournamehere-{random}-" padded out to exactly 30 characters.
	if len(cfg.BucketPrefix) != maxPrefixLen {
		t.Errorf("BucketPrefix = %q, len %d, want %d", cfg.BucketPrefix, len(cfg.BucketPrefix), maxPrefixLen)
	}
	if !strings.HasPrefix(cfg.BucketPrefix, "yournamehere-") {
		t.Errorf("BucketPrefix = %q, want the template prefix retained", cfg.BucketPrefix)
	}
}

// The DEFAULT section is inherited, section names and keys contain spaces, and
// both '=' and ':' separate keys from values.
func TestParseINI(t *testing.T) {
	raw, err := parseINI(strings.NewReader(`
; leading comment before any section
host = example.com
port = 8000

[s3 main]
access_key = AAAA
display_name : M. Tester
# comment
secret_key = with#hash

[fixtures]
bucket prefix = pfx-{random}-
`))
	if err != nil {
		t.Fatalf("parseINI: %v", err)
	}

	for _, tc := range []struct{ section, key, want string }{
		{"s3 main", "access_key", "AAAA"},
		{"s3 main", "display_name", "M. Tester"},
		{"s3 main", "host", "example.com"}, // inherited from DEFAULT
		{"fixtures", "bucket prefix", "pfx-{random}-"},
		{"fixtures", "port", "8000"}, // inherited from DEFAULT
		// RawConfigParser does not strip inline comments.
		{"s3 main", "secret_key", "with#hash"},
	} {
		got, ok := raw.get(tc.section, tc.key)
		if !ok {
			t.Errorf("get(%q, %q) missing", tc.section, tc.key)
			continue
		}
		if got != tc.want {
			t.Errorf("get(%q, %q) = %q, want %q", tc.section, tc.key, got, tc.want)
		}
	}

	if raw.has("nonexistent") {
		t.Error("has(nonexistent) = true")
	}
	if _, ok := raw.get("s3 main", "nope"); ok {
		t.Error("get returned a value for a missing key")
	}
}

func TestBuildRequiresSections(t *testing.T) {
	raw, err := parseINI(strings.NewReader("host = h\nport = 1\n[s3 main]\n"))
	if err != nil {
		t.Fatalf("parseINI: %v", err)
	}
	if _, err := build(raw); err == nil {
		t.Fatal("build accepted a config with no 's3 alt' section")
	}
}

func TestBucketPrefix(t *testing.T) {
	t.Run("padded to max", func(t *testing.T) {
		got, err := BucketPrefix("pfx-{random}-")
		if err != nil {
			t.Fatalf("BucketPrefix: %v", err)
		}
		if len(got) != maxPrefixLen {
			t.Errorf("len(%q) = %d, want %d", got, len(got), maxPrefixLen)
		}
		if !strings.HasPrefix(got, "pfx-") || !strings.HasSuffix(got, "-") {
			t.Errorf("BucketPrefix = %q, want literal parts preserved", got)
		}
	})

	t.Run("no placeholder", func(t *testing.T) {
		got, err := BucketPrefix("fixed-")
		if err != nil {
			t.Fatalf("BucketPrefix: %v", err)
		}
		if got != "fixed-" {
			t.Errorf("BucketPrefix = %q, want fixed-", got)
		}
	})

	t.Run("template too long", func(t *testing.T) {
		if _, err := BucketPrefix(strings.Repeat("x", 31) + "{random}"); err == nil {
			t.Error("BucketPrefix accepted a template that cannot fit")
		}
	})

	t.Run("random differs", func(t *testing.T) {
		a, _ := BucketPrefix("p-{random}-")
		b, _ := BucketPrefix("p-{random}-")
		if a == b {
			t.Errorf("two prefixes are identical: %q", a)
		}
	})
}
