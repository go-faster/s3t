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
	out = append(out, objectsTests(b)...)
	out = append(out, copyTests(b)...)
	out = append(out, multipartTests(b)...)
	out = append(out, taggingTests(b)...)
	out = append(out, conditionalTests(b)...)
	out = append(out, conditionalDeleteTests(b)...)
	out = append(out, miscTests(b)...)
	out = append(out, atomicTests(b)...)
	out = append(out, encryptionTests(b)...)
	out = append(out, aclTests(b)...)
	out = append(out, lockTests(b)...)
	out = append(out, objectLockTests(b)...)
	out = append(out, presignedTests(b)...)
	out = append(out, versioningTests(b)...)
	out = append(out, sseS3Tests(b)...)
	out = append(out, checksumTests(b)...)
	out = append(out, multipartChecksumTests(b)...)
	out = append(out, bucketACLTests(b)...)
	out = append(out, accessTests(b)...)
	out = append(out, versionedTests(b)...)
	out = append(out, objectTests(b)...)
	return out
}

// mustField reads a response field the test needs, failing when the server
// omitted it.
//
// Upstream indexes the response dict -- response['VersionId'], response
// ['Marker'] -- so an omitted field raises KeyError and the test fails right
// there. aws.ToString would turn the same omission into an empty string and
// let the test carry on to pass, which is a port bug rather than a stricter
// port: the two suites have to fail on the same servers.
func mustField(e *fixture.Env, v *string, what string) string {
	if v == nil {
		e.T.Fatalf("response carries no %s", what)
	}
	return *v
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
