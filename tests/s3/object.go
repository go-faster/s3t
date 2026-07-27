package s3

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func objectTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_write_check_etag", objectWriteCheckEtag),
		b.add("object_write_cache_control", objectWriteCacheControl),
		b.add("object_head_zero_bytes", objectHeadZeroBytes),
		b.add("object_read_not_exist", objectReadNotExist),
	}
}

func objectWriteCheckEtag(e *fixture.Env) {
	bucket := e.NewBucket()

	out, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   strings.NewReader("bar"),
	})
	s3util.NoError(e.T, err, "put object")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
	s3util.Equal(e.T, aws.ToString(out.ETag), `"37b51d194a7513e45b56f6524f2d51f2"`, "etag")
}

func objectWriteCacheControl(e *fixture.Env) {
	bucket := e.NewBucket()
	const cacheControl = "public, max-age=14400"

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String("foo"),
		Body:         strings.NewReader("bar"),
		CacheControl: aws.String(cacheControl),
	})
	s3util.NoError(e.T, err, "put object")

	out, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, aws.ToString(out.CacheControl), cacheControl, "cache-control")
}

func objectHeadZeroBytes(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   strings.NewReader(""),
	})
	s3util.NoError(e.T, err, "put object")

	out, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, aws.ToInt64(out.ContentLength), 0, "content length")
}

func objectReadNotExist(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("bar"),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
}

// createObjects creates a bucket holding the given keys, each with its name as
// its contents, mirroring upstream's _create_objects.
func createObjects(e *fixture.Env, keys ...string) string {
	bucket := e.NewBucket()
	for _, key := range keys {
		_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   strings.NewReader(key),
		})
		s3util.NoError(e.T, err, "put object "+key)
	}
	return bucket
}
