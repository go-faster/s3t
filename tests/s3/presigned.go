package s3

import (
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func presignedTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_presigned_put_object_with_acl", presignedPutWithACL(false)),
		b.add("object_presigned_put_object_with_acl_tenant", presignedPutWithACL(true)),
	}
}

// presignGet returns a presigned URL for reading a key.
func presignGet(e *fixture.Env, bucket, key string) string {
	req, err := e.Presign().PresignGetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "presign get object")
	return req.URL
}

// presignedPutWithACL writes through a presigned PUT carrying a canned ACL and
// reads it back through a presigned GET, mirroring upstream's
// _test_object_presigned_put_object_with_acl.
//
// The x-amz-acl header has to be signed into the URL and repeated on the
// request: a presigned URL commits to the exact set of headers the sender will
// use, so sending an unsigned one, or omitting a signed one, is rejected.
func presignedPutWithACL(tenant bool) func(*fixture.Env) {
	return func(e *fixture.Env) {
		presign := e.Presign()
		bucket := e.NewBucket()
		if tenant {
			presign = e.PresignTenant()
			bucket = e.NewBucketFor(e.TenantClient())
		}
		const key = "foo"
		const body = "hello world"

		put, err := presign.PresignPutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			ACL:    "private",
		})
		s3util.NoError(e.T, err, "presign put object")

		req, err := http.NewRequestWithContext(e.Ctx(), http.MethodPut, put.URL, readerOf(body))
		s3util.NoError(e.T, err, "build presigned put request")
		for name, values := range put.SignedHeader {
			for _, v := range values {
				req.Header.Add(name, v)
			}
		}

		resp, err := e.HTTP().Do(req)
		s3util.NoError(e.T, err, "send presigned put")
		_ = resp.Body.Close()
		s3util.Equal(e.T, resp.StatusCode, http.StatusOK, "put status")

		getReq, err := presign.PresignGetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		s3util.NoError(e.T, err, "presign get object")

		got, err := e.HTTP().Get(getReq.URL) //nolint:noctx // presigned URL, bounded by the client timeout
		s3util.NoError(e.T, err, "send presigned get")
		defer func() { _ = got.Body.Close() }()

		s3util.Equal(e.T, got.StatusCode, http.StatusOK, "get status")
		s3util.Equal(e.T, readAll(e, got.Body), body, "body")
	}
}
