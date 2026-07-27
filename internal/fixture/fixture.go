// Package fixture provides the per-test environment: clients, bucket naming
// and cleanup.
package fixture

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
	"github.com/go-faster/s3t/internal/harness"
)

// Env is what a test body works with: the clients it needs and buckets that
// clean themselves up.
type Env struct {
	T   *harness.T
	Cfg *config.Config

	clients *client.Factory
	prefix  string
	n       int
}

// New builds the environment for one test.
//
// Each test gets its own bucket prefix so cleanup is scoped to the test that
// created the buckets. Upstream instead deletes every bucket matching one
// global prefix before and after every test, which forces a serial run; this
// is what makes concurrency safe here.
func New(t *harness.T, cfg *config.Config, clients *client.Factory) *Env {
	return &Env{
		T:       t,
		Cfg:     cfg,
		clients: clients,
		prefix:  cfg.BucketPrefix + slug(t.Name()) + "-" + token() + "-",
	}
}

// Client returns the client for the "s3 main" user.
func (e *Env) Client() *s3.Client { return e.clients.Main() }

// AltClient returns the client for the "s3 alt" user.
func (e *Env) AltClient() *s3.Client { return e.clients.Alt() }

// Ctx returns the test context.
func (e *Env) Ctx() context.Context { return e.T.Ctx() }

// Prefix returns the bucket prefix unique to this test.
func (e *Env) Prefix() string { return e.prefix }

// NewBucketName returns an unused bucket name without creating it.
func (e *Env) NewBucketName() string {
	e.n++
	return fmt.Sprintf("%s%d", e.prefix, e.n)
}

// NewBucket creates an empty bucket and registers its removal.
func (e *Env) NewBucket() string {
	return e.NewBucketFor(e.Client())
}

// NewBucketFor creates an empty bucket owned by the given client's user.
func (e *Env) NewBucketFor(c *s3.Client) string {
	name := e.NewBucketName()
	err := retryThrottled(e.Ctx(), func() error {
		_, err := c.CreateBucket(e.Ctx(), &s3.CreateBucketInput{
			Bucket: aws.String(name),
		})
		return err
	})
	if err != nil {
		e.T.Fatalf("create bucket %s: %v", name, err)
	}
	e.T.Cleanup(func() { e.nuke(c, name) })
	return name
}

// nuke empties and deletes a bucket, reporting problems without failing: a
// test that already passed should not be failed by a teardown race, but a
// silent leak would be worse.
func (e *Env) nuke(c *s3.Client, bucket string) {
	// Cleanup runs after the test context may already be done, so it gets a
	// fresh one; otherwise an interrupted or timed-out test leaks every
	// bucket it made.
	ctx := context.WithoutCancel(e.Ctx())

	if err := retryThrottled(ctx, func() error {
		return e.deleteObjects(ctx, c, bucket)
	}); err != nil {
		e.T.Logf("cleanup: empty bucket %s: %v", bucket, err)
	}
	if err := retryThrottled(ctx, func() error {
		_, err := c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
		return err
	}); err != nil {
		e.T.Logf("cleanup: delete bucket %s: %v", bucket, err)
	}
}

// deleteObjects removes every version and delete marker in the bucket.
//
// Listing versions rather than objects handles versioned and unversioned
// buckets alike: an unversioned bucket reports its objects with a "null"
// version, so one code path covers both.
func (e *Env) deleteObjects(ctx context.Context, c *s3.Client, bucket string) error {
	in := &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)}
	for {
		page, err := c.ListObjectVersions(ctx, in)
		if err != nil {
			return err
		}

		ids := make([]types.ObjectIdentifier, 0, len(page.Versions)+len(page.DeleteMarkers))
		for _, v := range page.Versions {
			ids = append(ids, types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
		}
		for _, m := range page.DeleteMarkers {
			ids = append(ids, types.ObjectIdentifier{Key: m.Key, VersionId: m.VersionId})
		}
		if len(ids) > 0 {
			if _, err := c.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
			}); err != nil {
				return err
			}
		}

		if !aws.ToBool(page.IsTruncated) {
			return nil
		}
		in.KeyMarker = page.NextKeyMarker
		in.VersionIdMarker = page.NextVersionIdMarker
	}
}

// maxSlugLen bounds the test-name portion of a bucket name. The config prefix
// is up to 30 characters and S3 allows 63, so the rest of the name has to stay
// short; the slug is for humans reading a bucket listing, not for uniqueness.
const maxSlugLen = 16

func slug(name string) string {
	var b strings.Builder
	for _, r := range name {
		if b.Len() == maxSlugLen {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			// Bucket names allow no underscores, and a leading or
			// doubled dash is ugly rather than invalid, so drop them.
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// token distinguishes concurrent runs against the same server.
func token() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = alphabet[rand.IntN(len(alphabet))] //nolint:gosec // uniqueness only
	}
	return string(b)
}
