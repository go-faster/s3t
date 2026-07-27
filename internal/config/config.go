// Package config reads the s3tests.conf file used by ceph/s3-tests.
//
// The format is Python configparser's, and existing config files must work
// unchanged: that compatibility is the point of the package.
package config

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"github.com/go-faster/errors"
)

// Section names in s3tests.conf.
const (
	sectionMain   = "s3 main"
	sectionAlt    = "s3 alt"
	sectionTenant = "s3 tenant"
)

// User is one set of credentials from the config, corresponding to an
// "s3 main" / "s3 alt" / "s3 tenant" section.
type User struct {
	AccessKey   string
	SecretKey   string
	DisplayName string
	UserID      string
	Email       string
}

// Config is the parsed s3tests.conf.
//
// Fields are added as ported tests need them rather than mirroring every
// upstream option up front.
type Config struct {
	Endpoint  string
	Host      string
	Port      int
	IsSecure  bool
	SSLVerify bool

	// APIName is the zonegroup api_name, used as the client region.
	APIName string

	Main   User
	Alt    User
	Tenant User

	// TenantName is the tenant the "s3 tenant" user belongs to.
	TenantName string

	// BucketPrefix is resolved from the "bucket prefix" template, with
	// {random} already filled in.
	BucketPrefix string

	// KMSKeyID is the key the SSE-KMS tests name. Upstream defaults it to
	// testkey-1 when the config omits it.
	KMSKeyID string

	// SecondaryKMSKeyID is the second key, used where a test needs two
	// different ones.
	SecondaryKMSKeyID string
}

// Load reads and validates a config file.
func Load(path string) (*Config, error) {
	// The config path comes from the user, which is the entire point.
	f, err := os.Open(path) //nolint:gosec // caller-supplied config path
	if err != nil {
		return nil, errors.Wrap(err, "open config")
	}
	defer func() { _ = f.Close() }()

	raw, err := parseINI(f)
	if err != nil {
		return nil, errors.Wrap(err, "parse config")
	}
	return build(raw)
}

func build(raw *ini) (*Config, error) {
	// Upstream fails loudly when these are missing rather than running a
	// partial suite, and so do we.
	for _, s := range []string{sectionMain, sectionAlt, sectionTenant} {
		if !raw.has(s) {
			return nil, errors.Errorf("config is missing the %q section", s)
		}
	}

	cfg := &Config{}
	var err error
	if cfg.Host, err = required(raw, defaultSection, "host"); err != nil {
		return nil, err
	}
	if cfg.Port, err = requiredInt(raw, defaultSection, "port"); err != nil {
		return nil, err
	}
	cfg.IsSecure = boolean(raw, defaultSection, "is_secure", false)
	cfg.SSLVerify = boolean(raw, defaultSection, "ssl_verify", false)

	scheme := "http"
	if cfg.IsSecure {
		scheme = "https"
	}
	cfg.Endpoint = fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port)

	cfg.APIName, _ = raw.get(sectionMain, "api_name")

	var ok bool
	if cfg.KMSKeyID, ok = raw.get(sectionMain, "kms_keyid"); !ok {
		cfg.KMSKeyID = "testkey-1"
	}
	if cfg.SecondaryKMSKeyID, ok = raw.get(sectionMain, "kms_keyid2"); !ok {
		cfg.SecondaryKMSKeyID = "testkey-2"
	}

	for _, u := range []struct {
		section string
		dst     *User
	}{
		{sectionMain, &cfg.Main},
		{sectionAlt, &cfg.Alt},
		{sectionTenant, &cfg.Tenant},
	} {
		if *u.dst, err = user(raw, u.section); err != nil {
			return nil, err
		}
	}
	if cfg.TenantName, err = required(raw, sectionTenant, "tenant"); err != nil {
		return nil, err
	}

	template, ok2 := raw.get("fixtures", "bucket prefix")
	if !ok2 {
		template = "test-{random}-"
	}
	if cfg.BucketPrefix, err = BucketPrefix(template); err != nil {
		return nil, err
	}
	return cfg, nil
}

func user(raw *ini, section string) (User, error) {
	var u User
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"access_key", &u.AccessKey},
		{"secret_key", &u.SecretKey},
		{"display_name", &u.DisplayName},
		{"user_id", &u.UserID},
		{"email", &u.Email},
	} {
		v, err := required(raw, section, f.key)
		if err != nil {
			return User{}, err
		}
		*f.dst = v
	}
	return u, nil
}

func required(raw *ini, section, key string) (string, error) {
	v, ok := raw.get(section, key)
	if !ok || v == "" {
		return "", errors.Errorf("config is missing %q in section %q", key, section)
	}
	return v, nil
}

func requiredInt(raw *ini, section, key string) (int, error) {
	v, err := required(raw, section, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errors.Errorf("config value %q in section %q is not a number: %q", key, section, v)
	}
	return n, nil
}

// boolean matches configparser's getboolean, which accepts 1/yes/true/on and
// 0/no/false/off, case-insensitively. An unparseable value falls back to def
// rather than failing, matching how upstream treats these options.
func boolean(raw *ini, section, key string, def bool) bool {
	v, ok := raw.get(section, key)
	if !ok {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "yes", "true", "on":
		return true
	case "0", "no", "false", "off":
		return false
	default:
		return def
	}
}

// maxPrefixLen caps the generated bucket prefix, leaving room for the
// per-test suffix within S3's 63-character bucket name limit.
const maxPrefixLen = 30

// BucketPrefix fills {random} in a prefix template with as much random filler
// as fits in maxPrefixLen, mirroring upstream's choose_bucket_prefix.
func BucketPrefix(template string) (string, error) {
	const placeholder = "{random}"
	fixed := len(strings.ReplaceAll(template, placeholder, ""))
	if fixed > maxPrefixLen {
		return "", errors.Errorf("bucket prefix template %q cannot fit in %d characters", template, maxPrefixLen)
	}
	if !strings.Contains(template, placeholder) {
		return template, nil
	}
	return strings.ReplaceAll(template, placeholder, randomString(maxPrefixLen-fixed)), nil
}

// randomString returns n lowercase alphanumeric characters. The values only
// need to avoid collisions between concurrent runs, not resist guessing.
func randomString(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.IntN(len(alphabet))] //nolint:gosec // collision avoidance only
	}
	return string(b)
}
