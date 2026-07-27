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

func namingTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_create_naming_bad_ip", bucketCreateNamingBadIP, "fails_on_aws"),
		b.add("bucket_create_naming_bad_short_one", bucketCreateNamingBadShortOne),
		b.add("bucket_create_naming_bad_short_two", bucketCreateNamingBadShortTwo),
		b.add("bucket_create_naming_bad_starts_nonalpha", bucketCreateNamingBadStartsNonalpha),
		b.add("bucket_create_naming_dns_dash_at_end", bucketCreateNamingDNSDashAtEnd),
		b.add("bucket_create_naming_dns_dash_dot", bucketCreateNamingDNSDashDot),
		b.add("bucket_create_naming_dns_dot_dash", bucketCreateNamingDNSDotDash),
		b.add("bucket_create_naming_dns_dot_dot", bucketCreateNamingDNSDotDot),
		b.add("bucket_create_naming_dns_long", bucketCreateNamingDNSLong, "fails_on_aws"),
		b.add("bucket_create_naming_dns_underscore", bucketCreateNamingDNSUnderscore),
		b.add("bucket_create_naming_good_contains_hyphen", bucketCreateNamingGoodContainsHyphen),
		b.add("bucket_create_naming_good_contains_period", bucketCreateNamingGoodContainsPeriod),
		b.add("bucket_create_naming_good_long_60", bucketCreateNamingGoodLong60),
		b.add("bucket_create_naming_good_long_61", bucketCreateNamingGoodLong61),
		b.add("bucket_create_naming_good_long_62", bucketCreateNamingGoodLong62),
		b.add("bucket_create_naming_good_long_63", bucketCreateNamingGoodLong63),
		b.add("bucket_create_naming_good_starts_alpha", bucketCreateNamingGoodStartsAlpha),
		b.add("bucket_create_naming_good_starts_digit", bucketCreateNamingGoodStartsDigit),
		b.add("bucket_create_special_key_names", bucketCreateSpecialKeyNames, "fails_on_dbstore"),
	}
}

// checkBadBucketName asserts the server rejects a name, mirroring upstream's
// check_bad_bucket_name.
//
// The name is sent by rewriting the request path rather than passing it as a
// parameter. Upstream has the same problem with botocore and solves it the same
// way: an SDK that validates the name locally would never put it on the wire,
// and the test is about what the *server* does.
func checkBadBucketName(e *fixture.Env, name string) {
	placeholder := e.NewBucketName()

	_, err := e.Client().CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket: aws.String(placeholder),
	}, client.WithPathReplace(placeholder, name))
	s3util.ErrorIs(e.T, err, 400, "InvalidBucketName")
}

// checkGoodBucketName asserts the server accepts prefix+name.
func checkGoodBucketName(e *fixture.Env, name string) {
	bucket := e.NewBucketNamed(e.Cfg.BucketPrefix + name)
	_ = bucket
}

// createNamingGoodLong creates a bucket whose whole name is length characters.
func createNamingGoodLong(e *fixture.Env, length int) {
	prefix := e.NewBucketName()
	if len(prefix) >= length {
		e.T.Fatalf("generated prefix %q is already %d characters, want under %d",
			prefix, len(prefix), length)
	}
	e.NewBucketNamed(prefix + strings.Repeat("a", length-len(prefix)))
}

func bucketCreateNamingBadIP(e *fixture.Env)       { checkBadBucketName(e, "192.168.5.123") }
func bucketCreateNamingBadShortOne(e *fixture.Env) { checkBadBucketName(e, "a") }
func bucketCreateNamingBadShortTwo(e *fixture.Env) { checkBadBucketName(e, "aa") }

func bucketCreateNamingBadStartsNonalpha(e *fixture.Env) {
	checkBadBucketName(e, "_"+e.NewBucketName())
}

func bucketCreateNamingDNSDashAtEnd(e *fixture.Env)  { checkBadBucketName(e, "foo-") }
func bucketCreateNamingDNSDashDot(e *fixture.Env)    { checkBadBucketName(e, "foo-.bar") }
func bucketCreateNamingDNSDotDash(e *fixture.Env)    { checkBadBucketName(e, "foo.-bar") }
func bucketCreateNamingDNSDotDot(e *fixture.Env)     { checkBadBucketName(e, "foo..bar") }
func bucketCreateNamingDNSUnderscore(e *fixture.Env) { checkBadBucketName(e, "foo_bar") }

func bucketCreateNamingDNSLong(e *fixture.Env) {
	prefix := e.Cfg.BucketPrefix
	s3util.EqualNow(e.T, len(prefix) < 50, true, "prefix is shorter than 50")
	checkGoodBucketName(e, strings.Repeat("a", 63-len(prefix)))
}

func bucketCreateNamingGoodContainsHyphen(e *fixture.Env) { checkGoodBucketName(e, "aaa-111") }
func bucketCreateNamingGoodContainsPeriod(e *fixture.Env) { checkGoodBucketName(e, "aaa.111") }
func bucketCreateNamingGoodStartsAlpha(e *fixture.Env)    { checkGoodBucketName(e, "foo") }
func bucketCreateNamingGoodStartsDigit(e *fixture.Env)    { checkGoodBucketName(e, "0bar") }

func bucketCreateNamingGoodLong60(e *fixture.Env) { createNamingGoodLong(e, 60) }
func bucketCreateNamingGoodLong61(e *fixture.Env) { createNamingGoodLong(e, 61) }
func bucketCreateNamingGoodLong62(e *fixture.Env) { createNamingGoodLong(e, 62) }
func bucketCreateNamingGoodLong63(e *fixture.Env) { createNamingGoodLong(e, 63) }

func bucketCreateSpecialKeyNames(e *fixture.Env) {
	keys := []string{" ", `"`, "$", "%", "&", "'", "<", ">", "_", "_ ", "_ _", "__"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")

	for _, name := range keys {
		body := getObjectBody(e, bucket, name)
		s3util.Equal(e.T, body, name, "body of "+name)

		_, err := e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(name),
			ACL:    types.ObjectCannedACLPrivate,
		})
		s3util.NoError(e.T, err, "put object acl "+name)
	}
}

// getObjectBody reads an object and returns its contents, mirroring upstream's
// _get_body.
func getObjectBody(e *fixture.Env, bucket, key string) string {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "get object "+key)
	defer func() { _ = out.Body.Close() }()

	return readAll(e, out.Body)
}
