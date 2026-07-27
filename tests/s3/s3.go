// Package s3 is the port of s3tests/functional/test_s3.py.
package s3

import (
	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
)

// Tests returns every test ported from test_s3.py.
//
// Tests are returned rather than registered from init() so there is no global
// state and no import-order dependency; the runner collects them.
func Tests(cfg *config.Config, clients *client.Factory) []harness.Test {
	b := builder{cfg: cfg, clients: clients}
	var out []harness.Test
	out = append(out, bucketTests(b)...)
	out = append(out, listingTests(b)...)
	out = append(out, listingV2Tests(b)...)
	out = append(out, namingTests(b)...)
	out = append(out, objectTests(b)...)
	return out
}

// builder turns a test body into a harness.Test, giving each its own
// environment.
type builder struct {
	cfg     *config.Config
	clients *client.Factory
}

// add builds a test. name is the upstream Python name without "test_", and
// markers are its pytest markers.
func (b builder) add(name string, fn func(*fixture.Env), markers ...string) harness.Test {
	return harness.Test{
		Name:    name,
		Module:  harness.ModuleS3,
		Markers: markers,
		Fn: func(t *harness.T) {
			fn(fixture.New(t, b.cfg, b.clients))
		},
	}
}
