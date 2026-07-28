package s3

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func conditionalTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("get_object_ifmodifiedsince_failed", getObjectIfmodifiedsinceFailed, "fails_on_dbstore"),
		b.add("get_object_ifunmodifiedsince_good", getObjectIfunmodifiedsinceGood, "fails_on_dbstore"),
		b.add("get_object_ifunmodifiedsince_failed", getObjectIfunmodifiedsinceFailed),
		b.add("put_object_ifmatch_good", putObjectIfmatchGood, "fails_on_aws"),
		b.add("put_object_ifmatch_failed", putObjectIfmatchFailed, "fails_on_dbstore"),
		b.add("put_object_ifmatch_overwrite_existed_good", putObjectIfmatchOverwriteExistedGood, "fails_on_aws"),
		b.add("put_object_ifnonmatch_good", putObjectIfnonmatchGood, "fails_on_aws"),
		b.add("put_object_ifnonmatch_failed", putObjectIfnonmatchFailed, "fails_on_aws", "fails_on_dbstore"),
		b.add("put_object_ifnonmatch_nonexisted_good", putObjectIfnonmatchNonexistedGood, "fails_on_aws"),
		b.add("put_object_ifnonmatch_overwrite_existed_failed", putObjectIfnonmatchOverwriteExistedFailed, "fails_on_aws", "fails_on_dbstore"),
	}
}

// conditionalKey is the key every conditional test writes, as upstream does.
const conditionalKey = "foo"

// putWithCondition writes an object carrying a conditional header.
//
// The header is added before signing, matching boto3's before-call hook, so the
// request the server validates is the one the test intends.
func putWithCondition(e *fixture.Env, bucket, body, header, value string) error {
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(conditionalKey),
		Body:   strings.NewReader(body),
	}, client.WithHeaders(map[string]string{header: value}))
	return err
}

func getObjectIfmodifiedsinceFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object")
	lastModified := aws.ToTime(out.LastModified)
	_ = out.Body.Close()

	// A second past the stored mtime, so the object is definitely not
	// modified since. Upstream sleeps for the same reason: the header has
	// one-second resolution.
	after := lastModified.Add(time.Second)
	time.Sleep(time.Second)

	_, err = e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String("foo"),
		IfModifiedSince: aws.Time(after),
	})
	s3util.ErrorStatus(e.T, err, 304)
}

func getObjectIfunmodifiedsinceGood(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	// The object was modified after 1994, so the condition fails.
	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String("foo"),
		IfUnmodifiedSince: aws.Time(mustParseHTTPDate(e, "Sat, 29 Oct 1994 19:43:31 GMT")),
	})
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
}

func getObjectIfunmodifiedsinceFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String("foo"),
		IfUnmodifiedSince: aws.Time(mustParseHTTPDate(e, "Sat, 29 Oct 2100 19:43:31 GMT")),
	})
	s3util.NoError(e.T, err, "get object if-unmodified-since")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, readAll(e, out.Body), "bar", "body")
}

func putObjectIfmatchGood(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := strings.Trim(aws.ToString(putObject(e, bucket, "foo", "bar").ETag), `"`)

	s3util.NoError(e.T, putWithCondition(e, bucket, "zar", "If-Match", etag), "put with if-match")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "zar", "body")
}

func putObjectIfmatchFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	err := putWithCondition(e, bucket, "zar", "If-Match", `"ABCORZ"`)
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	// The rejected write must not have replaced the object.
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")
}

func putObjectIfmatchOverwriteExistedGood(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	// "*" matches any existing object.
	s3util.NoError(e.T, putWithCondition(e, bucket, "zar", "If-Match", "*"), "put with if-match *")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "zar", "body")
}

func putObjectIfnonmatchGood(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	s3util.NoError(e.T, putWithCondition(e, bucket, "zar", "If-None-Match", "ABCORZ"),
		"put with if-none-match")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "zar", "body")
}

func putObjectIfnonmatchFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := strings.Trim(aws.ToString(putObject(e, bucket, "foo", "bar").ETag), `"`)

	err := putWithCondition(e, bucket, "zar", "If-None-Match", etag)
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")
}

func putObjectIfnonmatchNonexistedGood(e *fixture.Env) {
	bucket := e.NewBucket()

	// "*" against a key that does not exist yet succeeds.
	s3util.NoError(e.T, putWithCondition(e, bucket, "bar", "If-None-Match", "*"),
		"put with if-none-match *")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")
}

func putObjectIfnonmatchOverwriteExistedFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	// "*" against a key that already exists is refused.
	err := putWithCondition(e, bucket, "zar", "If-None-Match", "*")
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")
}
