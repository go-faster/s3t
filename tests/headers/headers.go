// Package headers is the port of s3tests/functional/test_headers.py.
package headers

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// Tests returns every test ported from test_headers.py.
func Tests(cfg *config.Config, clients *client.Factory) []harness.Test {
	b := builder{cfg: cfg, clients: clients}
	var out []harness.Test
	out = append(out, commonTests(b)...)
	out = append(out, aws2Tests(b)...)
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
		Module:  harness.ModuleHeaders,
		Markers: markers,
		Fn: func(t *harness.T) {
			fn(fixture.New(t, b.cfg, b.clients))
		},
	}
}

// key and content are the object every test in this file writes, as upstream.
const (
	key     = "foo"
	content = "bar"
)

// createObject creates a bucket and writes an empty object to it with the
// given request options, mirroring upstream's _add_header_create_object and
// _remove_header_create_object. The put has to succeed.
//
// Upstream registers the header hook on a throwaway client and the test then
// writes the object again through a clean one, so the options apply to this
// call only. c is the client the upstream helper takes as its second argument,
// which is how the auth_aws2 tests reach the v2 signer.
func createObject(e *fixture.Env, c *awss3.Client, opts ...func(*awss3.Options)) (bucket string) {
	// The bucket comes from the default client, as upstream's
	// get_new_bucket(): only the write under test uses c.
	bucket = e.NewBucket()
	_, err := c.PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, opts...)
	s3util.NoError(e.T, err, "put object")
	return bucket
}

// createBadObject is createObject for the calls that must fail. The body is
// non-empty, as upstream's _add_header_create_bad_object, which is what makes
// the length and digest headers matter.
func createBadObject(e *fixture.Env, c *awss3.Client, opts ...func(*awss3.Options)) error {
	bucket := e.NewBucket()
	_, err := c.PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	}, opts...)
	return err
}

// createBucket creates a bucket with the given request options, mirroring
// _add_header_create_bucket and _remove_header_create_bucket.
func createBucket(e *fixture.Env, c *awss3.Client, opts ...func(*awss3.Options)) {
	name := e.NewBucketName()
	_, err := c.CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket: aws.String(name),
	}, opts...)
	e.T.Cleanup(func() { e.Nuke(e.Client(), name) })
	s3util.NoError(e.T, err, "create bucket")
}

// createBadBucket is createBucket for the calls that must fail.
func createBadBucket(e *fixture.Env, c *awss3.Client, opts ...func(*awss3.Options)) error {
	name := e.NewBucketName()
	_, err := c.CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket: aws.String(name),
	}, opts...)
	// The create may have reached the server before whatever made it fail,
	// so the bucket still gets cleaned up.
	e.T.Cleanup(func() { e.Nuke(e.Client(), name) })
	return err
}

// putObject writes the object through an unmodified client, the second half of
// every upstream test that starts with a header-modified create.
func putObject(e *fixture.Env, c *awss3.Client, bucket string) {
	_, err := c.PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	})
	s3util.NoError(e.T, err, "put object")
}
