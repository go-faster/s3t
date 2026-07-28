package headers

import (
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

// markerAuthCommon is on every test in this file, matching upstream. It selects
// the header tests that hold for any signature version.
const markerAuthCommon = "auth_common"

func commonTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_create_bad_md5_invalid_short", objectCreateBadMD5InvalidShort, markerAuthCommon),
		b.add("object_create_bad_md5_bad", objectCreateBadMD5Bad, markerAuthCommon),
		b.add("object_create_bad_md5_empty", objectCreateBadMD5Empty, markerAuthCommon),
		b.add("object_create_bad_md5_none", objectCreateBadMD5None, markerAuthCommon),
		b.add("object_create_bad_expect_mismatch", objectCreateBadExpectMismatch, markerAuthCommon),
		b.add("object_create_bad_expect_empty", objectCreateBadExpectEmpty, markerAuthCommon),
		b.add("object_create_bad_expect_none", objectCreateBadExpectNone, markerAuthCommon),
		b.add("object_create_bad_contentlength_empty", objectCreateBadContentlengthEmpty,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_create_bad_contentlength_negative", objectCreateBadContentlengthNegative,
			markerAuthCommon, "fails_on_mod_proxy_fcgi"),
		b.add("object_create_bad_contentlength_none", objectCreateBadContentlengthNone,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_create_bad_contenttype_invalid", objectCreateBadContenttypeInvalid, markerAuthCommon),
		b.add("object_create_bad_contenttype_empty", objectCreateBadContenttypeEmpty, markerAuthCommon),
		b.add("object_create_bad_contenttype_none", objectCreateBadContenttypeNone, markerAuthCommon),
		b.add("object_create_bad_authorization_empty", objectCreateBadAuthorizationEmpty,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_create_date_and_amz_date", objectCreateDateAndAmzDate,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_create_amz_date_and_no_date", objectCreateAmzDateAndNoDate,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_create_bad_authorization_none", objectCreateBadAuthorizationNone,
			markerAuthCommon, "fails_on_rgw"),
		b.add("bucket_create_contentlength_none", bucketCreateContentlengthNone,
			markerAuthCommon, "fails_on_rgw"),
		b.add("object_acl_create_contentlength_none", objectACLCreateContentlengthNone,
			markerAuthCommon, "fails_on_rgw"),
		b.add("bucket_put_bad_canned_acl", bucketPutBadCannedACL, markerAuthCommon),
		b.add("bucket_create_bad_expect_mismatch", bucketCreateBadExpectMismatch, markerAuthCommon),
		b.add("bucket_create_bad_expect_empty", bucketCreateBadExpectEmpty, markerAuthCommon),
		b.add("bucket_create_bad_contentlength_empty", bucketCreateBadContentlengthEmpty,
			markerAuthCommon, "fails_on_rgw"),
		b.add("bucket_create_bad_contentlength_negative", bucketCreateBadContentlengthNegative,
			markerAuthCommon, "fails_on_mod_proxy_fcgi"),
		b.add("bucket_create_bad_contentlength_none", bucketCreateBadContentlengthNone,
			markerAuthCommon, "fails_on_rgw"),
		b.add("bucket_create_bad_authorization_empty", bucketCreateBadAuthorizationEmpty,
			markerAuthCommon, "fails_on_rgw"),
		b.add("bucket_create_bad_authorization_none", bucketCreateBadAuthorizationNone,
			markerAuthCommon, "fails_on_rgw"),
	}
}

func objectCreateBadMD5InvalidShort(e *fixture.Env) {
	err := createBadObject(e, client.WithHeaders(map[string]string{"Content-MD5": "YWJyYWNhZGFicmE="}))
	s3util.ErrorIs(e.T, err, 400, "InvalidDigest")
}

func objectCreateBadMD5Bad(e *fixture.Env) {
	// A well-formed digest of something other than the body.
	err := createBadObject(e, client.WithHeaders(map[string]string{"Content-MD5": "rL0Y20xC+Fzt72VPzMSk2A=="}))
	s3util.ErrorIs(e.T, err, 400, "BadDigest")
}

func objectCreateBadMD5Empty(e *fixture.Env) {
	err := createBadObject(e, client.WithHeaders(map[string]string{"Content-MD5": ""}))
	s3util.ErrorIs(e.T, err, 400, "InvalidDigest")
}

func objectCreateBadMD5None(e *fixture.Env) {
	bucket := createObject(e, client.WithoutHeader("Content-MD5"))
	putObject(e, bucket)
}

func objectCreateBadExpectMismatch(e *fixture.Env) {
	bucket := createObject(e, client.WithHeaders(map[string]string{"Expect": "200"}))
	putObject(e, bucket)
}

func objectCreateBadExpectEmpty(e *fixture.Env) {
	bucket := createObject(e, client.WithHeaders(map[string]string{"Expect": ""}))
	putObject(e, bucket)
}

func objectCreateBadExpectNone(e *fixture.Env) {
	bucket := createObject(e, client.WithoutHeader("Expect"))
	putObject(e, bucket)
}

func objectCreateBadContentlengthEmpty(e *fixture.Env) {
	err := createBadObject(e, e.WithContentLength(""))
	s3util.ErrorStatus(e.T, err, 400)
}

func objectCreateBadContentlengthNegative(e *fixture.Env) {
	bucket := e.NewBucket()

	// Upstream turns the checksum calculation off here so the SDK does not
	// switch to STREAMING-UNSIGNED-PAYLOAD-TRAILER, which would replace the
	// length being tested. Ours is off for every client already.
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, e.WithContentLength("-1"))
	s3util.ErrorStatus(e.T, err, 400)
}

func objectCreateBadContentlengthNone(e *fixture.Env) {
	err := createBadObject(e, e.WithoutContentLength())
	s3util.ErrorIs(e.T, err, 411, "MissingContentLength")
}

func objectCreateBadContenttypeInvalid(e *fixture.Env) {
	bucket := createObject(e, client.WithHeaders(map[string]string{"Content-Type": "text/plain"}))
	putObject(e, bucket)
}

func objectCreateBadContenttypeEmpty(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(content),
		ContentType: aws.String(""),
	})
	s3util.NoError(e.T, err, "put object")
}

func objectCreateBadContenttypeNone(e *fixture.Env) {
	// Leaving ContentType unset keeps it out of the request entirely.
	bucket := e.NewBucket()
	putObject(e, bucket)
}

func objectCreateBadAuthorizationEmpty(e *fixture.Env) {
	err := createBadObject(e, client.WithHeaders(map[string]string{"Authorization": ""}))
	s3util.ErrorStatus(e.T, err, 403)
}

func objectCreateDateAndAmzDate(e *fixture.Env) {
	date := time.Now().UTC().Format(http.TimeFormat)
	bucket := createObject(e, client.WithHeaders(map[string]string{
		"Date":       date,
		"X-Amz-Date": date,
	}))
	putObject(e, bucket)
}

func objectCreateAmzDateAndNoDate(e *fixture.Env) {
	date := time.Now().UTC().Format(http.TimeFormat)
	bucket := createObject(e, client.WithHeaders(map[string]string{
		"Date":       "",
		"X-Amz-Date": date,
	}))
	putObject(e, bucket)
}

func objectCreateBadAuthorizationNone(e *fixture.Env) {
	err := createBadObject(e, client.WithoutHeader("Authorization"))
	s3util.ErrorStatus(e.T, err, 403)
}

func bucketCreateContentlengthNone(e *fixture.Env) {
	createBucket(e, e.WithoutContentLength())
}

func objectACLCreateContentlengthNone(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket)

	_, err := e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    types.ObjectCannedACLPublicRead,
	}, e.WithoutContentLength())
	s3util.NoError(e.T, err, "put object acl")
}

func bucketPutBadCannedACL(e *fixture.Env) {
	bucket := e.NewBucket()

	// The canned ACL is valid in the request the SDK builds and invalid in
	// the one that goes out, so the server is what rejects it.
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    types.BucketCannedACLPublicRead,
	}, client.WithHeaders(map[string]string{"x-amz-acl": "public-ready"}))
	s3util.ErrorStatus(e.T, err, 400)
}

func bucketCreateBadExpectMismatch(e *fixture.Env) {
	createBucket(e, client.WithHeaders(map[string]string{"Expect": "200"}))
}

func bucketCreateBadExpectEmpty(e *fixture.Env) {
	createBucket(e, client.WithHeaders(map[string]string{"Expect": ""}))
}

func bucketCreateBadContentlengthEmpty(e *fixture.Env) {
	err := createBadBucket(e, e.WithContentLength(""))
	s3util.ErrorStatus(e.T, err, 400)
}

func bucketCreateBadContentlengthNegative(e *fixture.Env) {
	err := createBadBucket(e, e.WithContentLength("-1"))
	s3util.ErrorStatus(e.T, err, 400)
}

func bucketCreateBadContentlengthNone(e *fixture.Env) {
	createBucket(e, e.WithoutContentLength())
}

func bucketCreateBadAuthorizationEmpty(e *fixture.Env) {
	err := createBadBucket(e, client.WithHeaders(map[string]string{"Authorization": ""}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketCreateBadAuthorizationNone(e *fixture.Env) {
	err := createBadBucket(e, client.WithoutHeader("Authorization"))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}
