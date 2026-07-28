package s3

import (
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func lockTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("set_multipart_tagging", setMultipartTagging, markerTagging),
		b.add("100_continue_error_retry", continueErrorRetry, "fails_on_rgw"),
		b.add("object_requestid_matches_header_on_error", objectRequestidMatchesHeaderOnError, "fails_on_dbstore"),
		b.add("object_write_with_chunked_transfer_encoding", objectWriteWithChunkedTransferEncoding),
	}
}

func setMultipartTagging(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket:  aws.String(bucket),
		Key:     aws.String(key),
		Tagging: aws.String("Hello=World&foo=bar"),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	part, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   created.UploadId,
		PartNumber: aws.Int32(1),
		Body:       readerOf("a"),
	})
	s3util.NoError(e.T, err, "upload part")

	out, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{{ETag: part.ETag, PartNumber: aws.Int32(1)}},
		},
	})
	s3util.NoError(e.T, err, "complete multipart upload")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")

	// The tagging given at create time applies to the completed object.
	tags := getObjectTagging(e, bucket, key)
	equalTags(e, tags, []types.Tag{
		{Key: aws.String("Hello"), Value: aws.String("World")},
		{Key: aws.String("foo"), Value: aws.String("bar")},
	}, "tag set")

	del, err := e.Client().DeleteObjectTagging(e.Ctx(), &awss3.DeleteObjectTaggingInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "delete object tagging")
	s3util.Equal(e.T, client.Status(del.ResultMetadata), 204, "status")
	s3util.Equal(e.T, len(getObjectTagging(e, bucket, key)), 0, "tag count after delete")
}

func continueErrorRetry(e *fixture.Env) {
	bucket := e.NewBucket()

	// PutObject uses Expect: 100-continue. A write to a bucket that does not
	// exist must answer 404 rather than continue, and the connection must
	// stay usable afterwards.
	//
	// The name is a fresh one that was never created rather than a suffixed
	// version of an existing bucket: the per-test prefix is long enough that
	// suffixing would exceed the 63-character limit and draw InvalidBucketName
	// instead of the NoSuchBucket the test is about.
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(e.NewBucketName()),
		Key:    aws.String("foo"),
		Body:   readerOf("bar"),
	})
	s3util.ErrorStatus(e.T, err, 404)

	// The second request goes over the same pooled connection.
	putObject(e, bucket, "foo", "bar")
}

func objectRequestidMatchesHeaderOnError(e *fixture.Env) {
	bucket := e.NewBucket()

	// The request id in the error document must match the one in the header.
	// Reading it needs the raw response body, which the SDK consumes, so this
	// goes over a presigned URL instead.
	url := presignGet(e, bucket, "bar")
	resp, err := e.HTTP().Get(url) //nolint:noctx // presigned URL, bounded by the client timeout
	s3util.NoError(e.T, err, "get presigned url")
	defer func() { _ = resp.Body.Close() }()

	s3util.EqualNow(e.T, resp.StatusCode, http.StatusNotFound, "status")
	body := readAll(e, resp.Body)

	headerID := resp.Header.Get("x-amz-request-id")
	s3util.Equal(e.T, headerID != "", true, "x-amz-request-id header is set")

	bodyID := betweenTags(body, "RequestId")
	s3util.Equal(e.T, bodyID != "", true, "error document carries a RequestId")
	s3util.Equal(e.T, bodyID, headerID, "request id")
}

// betweenTags returns the text of the first <tag>...</tag> in s. The error
// document is small and fixed in shape, so this avoids an XML decoder for one
// assertion.
func betweenTags(s, tag string) string {
	open, closing := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, closing)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func objectWriteWithChunkedTransferEncoding(e *fixture.Env) {
	bucket := e.NewBucket()

	out, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   readerOf("bar"),
	}, client.WithChunkedTransferEncoding())
	s3util.NoError(e.T, err, "put object chunked")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}
