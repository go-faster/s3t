package s3

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func objectsTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_write_file", objectWriteFile),
		b.add("object_write_read_update_read_delete", objectWriteReadUpdateReadDelete),
		b.add("object_write_to_nonexist_bucket", objectWriteToNonexistBucket),
		b.add("object_metadata_replaced_on_put", objectMetadataReplacedOnPut),
		b.add("object_set_get_metadata_none_to_good", objectSetGetMetadataNoneToGood),
		b.add("object_set_get_metadata_none_to_empty", objectSetGetMetadataNoneToEmpty),
		b.add("object_set_get_metadata_overwrite_to_empty", objectSetGetMetadataOverwriteToEmpty),
		b.add("ranged_request_response_code", rangedRequestResponseCode, "fails_on_dbstore"),
		b.add("ranged_big_request_response_code", rangedBigRequestResponseCode, "fails_on_dbstore"),
		b.add("ranged_request_skip_leading_bytes_response_code", rangedRequestSkipLeadingBytes, "fails_on_dbstore"),
		b.add("ranged_request_return_trailing_bytes_response_code", rangedRequestReturnTrailingBytes, "fails_on_dbstore"),
		b.add("ranged_request_invalid_range", rangedRequestInvalidRange),
		b.add("ranged_request_empty_object", rangedRequestEmptyObject),
		b.add("multi_object_delete", multiObjectDelete),
		b.add("multi_objectv2_delete", multiObjectV2Delete, markerV2),
		b.add("get_object_ifmatch_good", getObjectIfmatchGood),
		b.add("get_object_ifmatch_failed", getObjectIfmatchFailed),
		b.add("get_object_ifnonematch_good", getObjectIfnonematchGood),
		b.add("get_object_ifnonematch_failed", getObjectIfnonematchFailed),
		b.add("get_object_ifmodifiedsince_good", getObjectIfmodifiedsinceGood),
	}
}

// readerOf is shorthand for a body literal.
func readerOf(s string) io.Reader { return strings.NewReader(s) }

// putObject writes an object and fails the test if it errors.
func putObject(e *fixture.Env, bucket, key, body string) *awss3.PutObjectOutput {
	out, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(body),
	})
	s3util.NoError(e.T, err, "put object "+key)
	return out
}

func objectWriteFile(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")
}

func objectWriteReadUpdateReadDelete(e *fixture.Env) {
	bucket := e.NewBucket()

	putObject(e, bucket, "foo", "bar")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body")

	putObject(e, bucket, "foo", "soup")
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "soup", "body after update")

	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "delete object")
}

func objectWriteToNonexistBucket(e *fixture.Env) {
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String("whatchutalkinboutwillis"),
		Key:    aws.String("foo"),
		Body:   strings.NewReader("foo"),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func objectMetadataReplacedOnPut(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String("foo"),
		Body:     strings.NewReader("bar"),
		Metadata: map[string]string{"meta1": "bar"},
	})
	s3util.NoError(e.T, err, "put object with metadata")

	// Writing again without metadata must drop it rather than merge.
	putObject(e, bucket, "foo", "bar")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, len(out.Metadata), 0, "metadata count")
}

// setGetMetadata writes an object carrying one metadata value and reads it
// back, mirroring upstream's _set_get_metadata.
func setGetMetadata(e *fixture.Env, value, bucket string) string {
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String("foo"),
		Body:     strings.NewReader("bar"),
		Metadata: map[string]string{"meta1": value},
	})
	s3util.NoError(e.T, err, "put object with metadata")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	return out.Metadata["meta1"]
}

func objectSetGetMetadataNoneToGood(e *fixture.Env) {
	s3util.Equal(e.T, setGetMetadata(e, "mymeta", e.NewBucket()), "mymeta", "metadata")
}

func objectSetGetMetadataNoneToEmpty(e *fixture.Env) {
	s3util.Equal(e.T, setGetMetadata(e, "", e.NewBucket()), "", "metadata")
}

func objectSetGetMetadataOverwriteToEmpty(e *fixture.Env) {
	bucket := e.NewBucket()
	s3util.Equal(e.T, setGetMetadata(e, "oldmeta", bucket), "oldmeta", "metadata")
	s3util.Equal(e.T, setGetMetadata(e, "", bucket), "", "metadata after overwrite")
}

// getRange reads a byte range and returns the body, the Content-Range header
// and the HTTP status.
//
// other keys.
//
//nolint:unparam // key mirrors the operation; the copy tests read ranges of
func getRange(e *fixture.Env, bucket, key, rng string) (body, contentRange string, status int) {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Range:  aws.String(rng),
	})
	s3util.NoError(e.T, err, "get object range "+rng)
	defer func() { _ = out.Body.Close() }()

	return readAll(e, out.Body), aws.ToString(out.ContentRange), client.Status(out.ResultMetadata)
}

func rangedRequestResponseCode(e *fixture.Env) {
	const content = "testcontent"
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", content)

	body, contentRange, status := getRange(e, bucket, "testobj", "bytes=4-7")
	s3util.Equal(e.T, body, content[4:8], "body")
	s3util.Equal(e.T, contentRange, "bytes 4-7/11", "content-range")
	s3util.Equal(e.T, status, 206, "status")
}

func rangedBigRequestResponseCode(e *fixture.Env) {
	content := s3util.RandomString(8 * 1024 * 1024)
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", content)

	body, contentRange, status := getRange(e, bucket, "testobj", "bytes=3145728-5242880")
	s3util.Equal(e.T, body, content[3145728:5242881], "body")
	s3util.Equal(e.T, contentRange, "bytes 3145728-5242880/8388608", "content-range")
	s3util.Equal(e.T, status, 206, "status")
}

func rangedRequestSkipLeadingBytes(e *fixture.Env) {
	const content = "testcontent"
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", content)

	body, contentRange, status := getRange(e, bucket, "testobj", "bytes=4-")
	s3util.Equal(e.T, body, content[4:], "body")
	s3util.Equal(e.T, contentRange, "bytes 4-10/11", "content-range")
	s3util.Equal(e.T, status, 206, "status")
}

func rangedRequestReturnTrailingBytes(e *fixture.Env) {
	const content = "testcontent"
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", content)

	body, contentRange, status := getRange(e, bucket, "testobj", "bytes=-7")
	s3util.Equal(e.T, body, content[len(content)-7:], "body")
	s3util.Equal(e.T, contentRange, "bytes 4-10/11", "content-range")
	s3util.Equal(e.T, status, 206, "status")
}

func rangedRequestInvalidRange(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", "testcontent")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("testobj"),
		Range:  aws.String("bytes=40-50"),
	})
	s3util.ErrorIs(e.T, err, 416, "InvalidRange")
}

func rangedRequestEmptyObject(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "testobj", "")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("testobj"),
		Range:  aws.String("bytes=40-50"),
	})
	s3util.ErrorIs(e.T, err, 416, "InvalidRange")
}

// deleteObjects removes keys in one request and checks the report, mirroring
// the shape upstream asserts on.
func deleteObjectsBatch(e *fixture.Env, bucket string, keys []string) {
	ids := make([]types.ObjectIdentifier, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, types.ObjectIdentifier{Key: aws.String(k)})
	}

	out, err := e.Client().DeleteObjects(e.Ctx(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: ids},
	})
	s3util.NoError(e.T, err, "delete objects")
	s3util.Equal(e.T, len(out.Deleted), len(keys), "deleted count")
	s3util.Equal(e.T, len(out.Errors), 0, "error count")
}

func multiObjectDelete(e *fixture.Env) {
	keys := []string{"key0", "key1", "key2"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, len(out.Contents), 3, "object count")

	// Deleting twice must report the same success: a delete of an absent
	// key is not an error.
	for range 2 {
		deleteObjectsBatch(e, bucket, keys)
		out = listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
		s3util.Equal(e.T, len(out.Contents), 0, "object count after delete")
	}
}

func multiObjectV2Delete(e *fixture.Env) {
	keys := []string{"key0", "key1", "key2"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, len(out.Contents), 3, "object count")

	for range 2 {
		deleteObjectsBatch(e, bucket, keys)
		out = listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		s3util.Equal(e.T, len(out.Contents), 0, "object count after delete")
	}
}

func getObjectIfmatchGood(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := aws.ToString(putObject(e, bucket, "foo", "bar").ETag)

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:  aws.String(bucket),
		Key:     aws.String("foo"),
		IfMatch: aws.String(etag),
	})
	s3util.NoError(e.T, err, "get object if-match")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, readAll(e, out.Body), "bar", "body")
}

func getObjectIfmatchFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:  aws.String(bucket),
		Key:     aws.String("foo"),
		IfMatch: aws.String(`"ABCORZ"`),
	})
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
}

func getObjectIfnonematchGood(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := aws.ToString(putObject(e, bucket, "foo", "bar").ETag)

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String("foo"),
		IfNoneMatch: aws.String(etag),
	})
	// 304 carries no body, so only the status can be asserted.
	s3util.ErrorStatus(e.T, err, 304)
}

func getObjectIfnonematchFailed(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String("foo"),
		IfNoneMatch: aws.String("ABCORZ"),
	})
	s3util.NoError(e.T, err, "get object if-none-match")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, readAll(e, out.Body), "bar", "body")
}

func getObjectIfmodifiedsinceGood(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String("foo"),
		IfModifiedSince: aws.Time(mustParseHTTPDate(e, "Sat, 29 Oct 1994 19:43:31 GMT")),
	})
	s3util.NoError(e.T, err, "get object if-modified-since")
	defer func() { _ = out.Body.Close() }()
	s3util.Equal(e.T, readAll(e, out.Body), "bar", "body")
}

func mustParseHTTPDate(e *fixture.Env, s string) time.Time {
	t, err := http.ParseTime(s)
	if err != nil {
		e.T.Fatalf("parse date %q: %v", s, err)
	}
	return t
}

func readAll(e *fixture.Env, r io.Reader) string {
	b, err := io.ReadAll(r)
	s3util.NoError(e.T, err, "read body")
	return string(b)
}
