package config

import (
	"bufio"
	"io"
	"strings"

	"github.com/go-faster/errors"
)

// defaultSection is the section every other section inherits from, matching
// Python's configparser.
const defaultSection = "DEFAULT"

// ini is the subset of Python's RawConfigParser behavior that s3tests.conf
// relies on: a DEFAULT section inherited by all others, section names and keys
// containing spaces ("s3 alt", "bucket prefix"), and no value interpolation.
//
// Inline comments are deliberately not stripped: RawConfigParser does not
// strip them either, so a value like "prefix#1" must survive intact.
type ini struct {
	sections map[string]map[string]string
}

func parseINI(r io.Reader) (*ini, error) {
	cfg := &ini{sections: map[string]map[string]string{}}
	section := defaultSection
	cfg.sections[section] = map[string]string{}

	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, ";") {
			continue
		}
		if strings.HasPrefix(text, "[") {
			if !strings.HasSuffix(text, "]") {
				return nil, errors.Errorf("line %d: unterminated section header", line)
			}
			section = strings.TrimSpace(text[1 : len(text)-1])
			if _, ok := cfg.sections[section]; !ok {
				cfg.sections[section] = map[string]string{}
			}
			continue
		}
		key, value, ok := splitKeyValue(text)
		if !ok {
			return nil, errors.Errorf("line %d: expected key = value", line)
		}
		cfg.sections[section][key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, errors.Wrap(err, "read")
	}
	return cfg, nil
}

// splitKeyValue splits on the first '=' or ':', whichever comes first, the way
// configparser does. Keys are lowercased, since configparser matches them
// case-insensitively.
func splitKeyValue(s string) (key, value string, ok bool) {
	i := strings.IndexAny(s, "=:")
	if i < 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(s[:i]))
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(s[i+1:]), true
}

// has reports whether a section is present.
func (c *ini) has(section string) bool {
	_, ok := c.sections[section]
	return ok
}

// get returns a value from a section, falling back to DEFAULT.
func (c *ini) get(section, key string) (string, bool) {
	if v, ok := c.sections[section][key]; ok {
		return v, true
	}
	v, ok := c.sections[defaultSection][key]
	return v, ok
}
