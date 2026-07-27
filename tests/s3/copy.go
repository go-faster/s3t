package s3

import (
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerCopy is on every test in this file, matching upstream.
const markerCopy = "copy"

func copyTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_copy_zero_size", objectCopyZeroSize, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_same_bucket", objectCopySameBucket, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_verify_contenttype", objectCopyVerifyContenttype, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_to_itself", objectCopyToItself, markerCopy),
		b.add("object_copy_to_itself_with_metadata", objectCopyToItselfWithMetadata, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_diff_bucket", objectCopyDiffBucket, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_canned_acl", objectCopyCannedACL, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_retaining_metadata", objectCopyRetainingMetadata, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_replacing_metadata", objectCopyReplacingMetadata, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_bucket_not_found", objectCopyBucketNotFound, markerCopy),
		b.add("object_copy_key_not_found", objectCopyKeyNotFound, markerCopy),
		b.add("object_copy_16m", objectCopy16m, markerCopy, "fails_on_dbstore"),
		b.add("copy_object_ifmatch_good", copyObjectIfmatchGood, markerCopy, "fails_on_dbstore"),
		b.add("copy_object_ifnonematch_failed", copyObjectIfnonematchFailed, markerCopy, "fails_on_dbstore"),
	}
}

// copySource builds the CopySource value, which is a single "bucket/key" path
// rather than a pair of fields.
//
// The key is URL-encoded, as boto3 does when given a {Bucket, Key} dict: keys
// containing a space or a '?' are otherwise rejected, and a key containing '%'
// would be read as an escape by the server. Slashes stay literal, since they
// are ordinary characters inside a key.
func copySource(bucket, key string) *string {
	u := url.URL{Path: bucket + "/" + key}
	return aws.String(u.EscapedPath())
}

// copyObject copies an object and fails the test if it errors.
func copyObject(e *fixture.Env, in *awss3.CopyObjectInput) {
	_, err := e.Client().CopyObject(e.Ctx(), in)
	s3util.NoError(e.T, err, "copy object")
}

func objectCopyZeroSize(e *fixture.Env) {
	const key = "foo123bar"
	bucket := createObjects(e, key)
	putObject(e, bucket, key, "")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, key),
		Key:        aws.String("bar321foo"),
	})

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("bar321foo"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, aws.ToInt64(out.ContentLength), 0, "content length")
}

func objectCopySameBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo123bar", "foo")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "foo123bar"),
		Key:        aws.String("bar321foo"),
	})
	s3util.Equal(e.T, getObjectBody(e, bucket, "bar321foo"), "foo", "body")
}

func objectCopyVerifyContenttype(e *fixture.Env) {
	bucket := e.NewBucket()
	const contentType = "text/bla"

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String("foo123bar"),
		Body:        strings.NewReader("foo"),
		ContentType: aws.String(contentType),
	})
	s3util.NoError(e.T, err, "put object")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "foo123bar"),
		Key:        aws.String("bar321foo"),
	})

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("bar321foo"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, readAll(e, out.Body), "foo", "body")
	s3util.Equal(e.T, aws.ToString(out.ContentType), contentType, "content type")
}

func objectCopyToItself(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo123bar", "foo")

	// Copying onto itself without replacing metadata is not allowed.
	_, err := e.Client().CopyObject(e.Ctx(), &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "foo123bar"),
		Key:        aws.String("foo123bar"),
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidRequest")
}

func objectCopyToItselfWithMetadata(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo123bar", "foo")
	metadata := map[string]string{"foo": "bar"}

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		CopySource:        copySource(bucket, "foo123bar"),
		Key:               aws.String("foo123bar"),
		Metadata:          metadata,
		MetadataDirective: types.MetadataDirectiveReplace,
	})

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo123bar"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	s3util.EqualMetadata(e.T, out.Metadata, metadata, "metadata")
}

func objectCopyDiffBucket(e *fixture.Env) {
	source := e.NewBucket()
	dest := e.NewBucket()
	putObject(e, source, "foo123bar", "foo")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(dest),
		CopySource: copySource(source, "foo123bar"),
		Key:        aws.String("bar321foo"),
	})
	s3util.Equal(e.T, getObjectBody(e, dest, "bar321foo"), "foo", "body")
}

func objectCopyCannedACL(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo123bar", "foo")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "foo123bar"),
		Key:        aws.String("bar321foo"),
		ACL:        types.ObjectCannedACLPublicRead,
	})
	// The ACL is checked by reading as a different user.
	altGetObject(e, bucket, "bar321foo")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		CopySource:        copySource(bucket, "bar321foo"),
		Key:               aws.String("foo123bar"),
		ACL:               types.ObjectCannedACLPublicRead,
		Metadata:          map[string]string{"abc": "def"},
		MetadataDirective: types.MetadataDirectiveReplace,
	})
	altGetObject(e, bucket, "foo123bar")
}

// altGetObject reads an object as the "s3 alt" user, failing the test if it
// cannot.
func altGetObject(e *fixture.Env, bucket, key string) {
	out, err := e.AltClient().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "get object as alt user")
	_ = out.Body.Close()
}

// copySizes are the two object sizes upstream runs the metadata copy tests at,
// one below and one above the size where a server is likely to switch code
// paths.
var copySizes = []int{3, 1024 * 1024}

func objectCopyRetainingMetadata(e *fixture.Env) {
	for _, size := range copySizes {
		bucket := e.NewBucket()
		const contentType = "audio/ogg"
		metadata := map[string]string{"key1": "value1", "key2": "value2"}

		_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String("foo123bar"),
			Body:        strings.NewReader(strings.Repeat("\x00", size)),
			Metadata:    metadata,
			ContentType: aws.String(contentType),
		})
		s3util.NoError(e.T, err, "put object")

		copyObject(e, &awss3.CopyObjectInput{
			Bucket:     aws.String(bucket),
			CopySource: copySource(bucket, "foo123bar"),
			Key:        aws.String("bar321foo"),
		})

		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String("bar321foo"),
		})
		s3util.NoError(e.T, err, "get object")
		s3util.Equal(e.T, aws.ToString(out.ContentType), contentType, "content type")
		s3util.EqualMetadata(e.T, out.Metadata, metadata, "metadata")
		s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
		_ = out.Body.Close()
	}
}

func objectCopyReplacingMetadata(e *fixture.Env) {
	for _, size := range copySizes {
		bucket := e.NewBucket()

		_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String("foo123bar"),
			Body:        strings.NewReader(strings.Repeat("\x00", size)),
			Metadata:    map[string]string{"key1": "value1", "key2": "value2"},
			ContentType: aws.String("audio/ogg"),
		})
		s3util.NoError(e.T, err, "put object")

		metadata := map[string]string{"key3": "value3", "key2": "value2"}
		const contentType = "audio/mpeg"

		copyObject(e, &awss3.CopyObjectInput{
			Bucket:            aws.String(bucket),
			CopySource:        copySource(bucket, "foo123bar"),
			Key:               aws.String("bar321foo"),
			Metadata:          metadata,
			MetadataDirective: types.MetadataDirectiveReplace,
			ContentType:       aws.String(contentType),
		})

		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String("bar321foo"),
		})
		s3util.NoError(e.T, err, "get object")
		s3util.Equal(e.T, aws.ToString(out.ContentType), contentType, "content type")
		s3util.EqualMetadata(e.T, out.Metadata, metadata, "metadata")
		s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
		_ = out.Body.Close()
	}
}

func objectCopyBucketNotFound(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().CopyObject(e.Ctx(), &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket+"-fake", "foo123bar"),
		Key:        aws.String("bar321foo"),
	})
	s3util.ErrorStatus(e.T, err, 404)
}

func objectCopyKeyNotFound(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().CopyObject(e.Ctx(), &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "foo123bar"),
		Key:        aws.String("bar321foo"),
	})
	s3util.ErrorStatus(e.T, err, 404)
}

func objectCopy16m(e *fixture.Env) {
	bucket := e.NewBucket()
	const size = 16 * 1024 * 1024

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("obj1"),
		Body:   strings.NewReader(strings.Repeat("\x00", size)),
	})
	s3util.NoError(e.T, err, "put object")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, "obj1"),
		Key:        aws.String("obj2"),
	})

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("obj2"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
}

func copyObjectIfmatchGood(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := aws.ToString(putObject(e, bucket, "foo", "bar").ETag)

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		CopySource:        copySource(bucket, "foo"),
		CopySourceIfMatch: aws.String(etag),
		Key:               aws.String("bar"),
	})
	s3util.Equal(e.T, getObjectBody(e, bucket, "bar"), "bar", "body")
}

func copyObjectIfnonematchFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:                aws.String(bucket),
		CopySource:            copySource(bucket, "foo"),
		CopySourceIfNoneMatch: aws.String("ABCORZ"),
		Key:                   aws.String("bar"),
	})
	s3util.Equal(e.T, getObjectBody(e, bucket, "bar"), "bar", "body")
}
