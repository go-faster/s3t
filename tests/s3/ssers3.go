package s3

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// Markers upstream puts on the SSE-S3 tests.
const (
	markerSSES3            = "sse_s3"
	markerBucketEncryption = "bucket_encryption"
)

func sseS3Tests(b builder) []harness.Test {
	var out []harness.Test

	for _, sz := range []struct {
		name string
		size int
	}{
		{"1b", 1},
		{"1kb", 1024},
		{"1mb", 1024 * 1024},
		{"8mb", 8 * 1024 * 1024},
	} {
		out = append(out,
			b.add("sse_s3_encrypted_upload_"+sz.name, sseS3EncryptedUpload(sz.size),
				markerEncryption, markerSSES3, "fails_on_dbstore"),
			b.add("sse_s3_default_upload_"+sz.name, sseS3DefaultUpload(sz.size),
				markerEncryption, markerBucketEncryption, markerSSES3, "fails_on_dbstore"),
		)
	}

	out = append(out,
		b.add("sse_s3_default_method_head", sseS3DefaultMethodHead,
			markerEncryption, markerBucketEncryption, markerSSES3, "fails_on_dbstore"),
		b.add("sse_s3_default_multipart_upload", sseS3DefaultMultipartUpload,
			markerEncryption, markerBucketEncryption, markerSSES3, "fails_on_dbstore"),
	)
	return out
}

// putBucketEncryptionS3 turns on default AES256 encryption for a bucket,
// mirroring upstream's _put_bucket_encryption_s3.
func putBucketEncryptionS3(e *fixture.Env, bucket string) {
	out, err := e.Client().PutBucketEncryption(e.Ctx(), &awss3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{{
				ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
					SSEAlgorithm: types.ServerSideEncryptionAes256,
				},
			}},
		},
	})
	s3util.NoError(e.T, err, "put bucket encryption")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}

// sseS3EncryptedUpload writes asking for SSE-S3 explicitly and reads it back.
func sseS3EncryptedUpload(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		data := strings.Repeat("A", size)

		put, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket:               aws.String(bucket),
			Key:                  aws.String(encKey),
			Body:                 strings.NewReader(data),
			ServerSideEncryption: types.ServerSideEncryptionAes256,
		})
		s3util.NoError(e.T, err, "put object")
		s3util.Equal(e.T, put.ServerSideEncryption, types.ServerSideEncryptionAes256,
			"server-side encryption on write")

		get, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(encKey),
		})
		s3util.NoError(e.T, err, "get object")
		defer func() { _ = get.Body.Close() }()
		s3util.Equal(e.T, get.ServerSideEncryption, types.ServerSideEncryptionAes256,
			"server-side encryption on read")
		s3util.Equal(e.T, readAll(e, get.Body) == data, true, "body")
	}
}

// sseS3DefaultUpload writes without asking for encryption to a bucket that
// encrypts by default.
func sseS3DefaultUpload(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		putBucketEncryptionS3(e, bucket)
		data := strings.Repeat("A", size)

		put, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(encKey),
			Body:   strings.NewReader(data),
		})
		s3util.NoError(e.T, err, "put object")
		s3util.Equal(e.T, put.ServerSideEncryption, types.ServerSideEncryptionAes256,
			"server-side encryption on write")

		get, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(encKey),
		})
		s3util.NoError(e.T, err, "get object")
		defer func() { _ = get.Body.Close() }()
		s3util.Equal(e.T, get.ServerSideEncryption, types.ServerSideEncryptionAes256,
			"server-side encryption on read")
		s3util.Equal(e.T, readAll(e, get.Body) == data, true, "body")
	}
}

func sseS3DefaultMethodHead(e *fixture.Env) {
	bucket := e.NewBucket()
	putBucketEncryptionS3(e, bucket)

	data := strings.Repeat("A", 1000)
	putObject(e, bucket, encKey, data)

	head, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(encKey),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, head.ServerSideEncryption, types.ServerSideEncryptionAes256,
		"server-side encryption")

	// Asking for SSE-S3 on a read is not allowed, even where it is what the
	// bucket does by default.
	_, err = e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(encKey),
	}, client.WithHeaders(map[string]string{"x-amz-server-side-encryption": "AES256"}))
	s3util.ErrorStatus(e.T, err, 400)
}

func sseS3DefaultMultipartUpload(e *fixture.Env) {
	bucket := e.NewBucket()
	putBucketEncryptionS3(e, bucket)

	const contentType = "text/plain"
	const size = 30 * 1024 * 1024
	metadata := map[string]string{"foo": "bar"}
	headers := map[string]string{"Content-Type": contentType}

	uploadID, data, parts := multipartEnc(e, bucket, encMultipartKey, size, defaultPartSize,
		headers, headers, metadata)

	_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(encMultipartKey),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	}, client.WithHeaders(headers))
	s3util.NoError(e.T, err, "complete multipart upload")

	listed := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(encMultipartKey),
	})
	s3util.EqualNow(e.T, len(listed.Contents), 1, "object count")
	s3util.Equal(e.T, aws.ToInt64(listed.Contents[0].Size), int64(size), "object size")

	get, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(encMultipartKey),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = get.Body.Close() }()

	s3util.EqualMetadata(e.T, get.Metadata, metadata, "metadata")
	s3util.Equal(e.T, aws.ToString(get.ContentType), contentType, "content type")
	s3util.Equal(e.T, get.ServerSideEncryption, types.ServerSideEncryptionAes256,
		"server-side encryption")
	s3util.Equal(e.T, readAll(e, get.Body) == data, true, "body")
}
