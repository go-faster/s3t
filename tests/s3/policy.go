package s3

import (
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerBucketPolicy is upstream's marker for the bucket-policy family.
const markerBucketPolicy = "bucket_policy"

func policyTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_policy", bucketPolicy, markerBucketPolicy),
		b.add("bucketv2_policy", bucketv2Policy, markerBucketPolicy, markerV2),
		b.add("bucket_policy_another_bucket", bucketPolicyAnotherBucket, markerBucketPolicy),
		b.add("bucketv2_policy_another_bucket", bucketv2PolicyAnotherBucket, markerBucketPolicy, markerV2),
		b.add("bucket_policy_set_condition_operator_end_with_IfExists",
			bucketPolicySetConditionOperatorEndWithIfExists, "fails_on_rgw"),
		b.add("get_bucket_policy_status", getBucketPolicyStatus),
	}
}

// policyDocument renders a one-statement bucket policy, mirroring upstream's
// json.dumps of a dict literal.
func policyDocument(e *fixture.Env, statement map[string]any) string {
	doc, err := json.Marshal(map[string]any{
		"Version":   "2012-10-17",
		"Statement": []any{statement},
	})
	s3util.NoError(e.T, err, "marshal policy")
	return string(doc)
}

// listBucketByAnyone is the statement most of these tests install: anyone may
// list the named resources.
func listBucketByAnyone(resources ...string) map[string]any {
	return map[string]any{
		"Effect":    "Allow",
		"Principal": map[string]any{"AWS": "*"},
		"Action":    "s3:ListBucket",
		"Resource":  resources,
	}
}

func putPolicy(e *fixture.Env, bucket, doc string) {
	_, err := e.Client().PutBucketPolicy(e.Ctx(), &awss3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(doc),
	})
	s3util.NoError(e.T, err, "put bucket policy")
}

func getPolicy(e *fixture.Env, bucket string) string {
	out, err := e.Client().GetBucketPolicy(e.Ctx(), &awss3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get bucket policy")
	return mustField(e, out.Policy, "Policy")
}

// bucketARN is the resource name of a bucket, and bucketARN+"/*" of everything
// in it.
func bucketARN(bucket string) string { return "arn:aws:s3:::" + bucket }

func bucketPolicy(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "asdf", "asdf")

	putPolicy(e, bucket, policyDocument(e,
		listBucketByAnyone(bucketARN(bucket), bucketARN(bucket)+"/*")))

	out, err := e.AltClient().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list objects as alt user")
	s3util.Equal(e.T, len(out.Contents), 1, "object count")
}

func bucketv2Policy(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "asdf", "asdf")

	putPolicy(e, bucket, policyDocument(e,
		listBucketByAnyone(bucketARN(bucket), bucketARN(bucket)+"/*")))

	out, err := e.AltClient().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list objects as alt user")
	s3util.Equal(e.T, len(out.Contents), 1, "object count")
}

// anotherBucketPolicy sets a wildcard policy on one bucket, reads it back, and
// installs the same document on a second one. Both tests below differ only in
// which listing API they then call as the alt user.
func anotherBucketPolicy(e *fixture.Env) (bucket1, bucket2 string) {
	bucket1, bucket2 = e.NewBucket(), e.NewBucket()
	putObject(e, bucket1, "asdf", "asdf")
	putObject(e, bucket2, "abcd", "abcd")

	putPolicy(e, bucket1, policyDocument(e,
		listBucketByAnyone("arn:aws:s3:::*", "arn:aws:s3:::*/*")))
	putPolicy(e, bucket2, getPolicy(e, bucket1))
	return bucket1, bucket2
}

func bucketPolicyAnotherBucket(e *fixture.Env) {
	bucket1, bucket2 := anotherBucketPolicy(e)

	for _, bucket := range []string{bucket1, bucket2} {
		out, err := e.AltClient().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
			Bucket: aws.String(bucket),
		})
		s3util.NoError(e.T, err, "list objects as alt user")
		s3util.Equal(e.T, len(out.Contents), 1, "object count in "+bucket)
	}
}

func bucketv2PolicyAnotherBucket(e *fixture.Env) {
	bucket1, bucket2 := anotherBucketPolicy(e)

	for _, bucket := range []string{bucket1, bucket2} {
		out, err := e.AltClient().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		s3util.NoError(e.T, err, "list objects as alt user")
		s3util.Equal(e.T, len(out.Contents), 1, "object count in "+bucket)
	}
}

func bucketPolicySetConditionOperatorEndWithIfExists(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "foo"
	putObject(e, bucket, key, "")

	// StringLikeIfExists lets a request with no Referer through and matches
	// the pattern when there is one.
	putPolicy(e, bucket, policyDocument(e, map[string]any{
		"Sid":       "Allow Public Access to All Objects",
		"Effect":    "Allow",
		"Principal": "*",
		"Action":    "s3:GetObject",
		"Condition": map[string]any{
			"StringLikeIfExists": map[string]any{"aws:Referer": "http://www.example.com/*"},
		},
		"Resource": bucketARN(bucket) + "/*",
	}))

	get := func(referer string) (*awss3.GetObjectOutput, error) {
		return e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}, client.WithHeaders(map[string]string{"referer": referer}))
	}

	for _, referer := range []string{"http://www.example.com/", "http://www.example.com/index.html"} {
		out, err := get(referer)
		s3util.NoError(e.T, err, "get object with referer "+referer)
		s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
		_ = out.Body.Close()
	}

	// This one does not match the pattern, so the condition denies it.
	_, err := get("http://example.com")
	s3util.ErrorStatus(e.T, err, 403)

	getPolicy(e, bucket)
}

func getBucketPolicyStatus(e *fixture.Env) {
	bucket := e.NewBucket()

	out, err := e.Client().GetBucketPolicyStatus(e.Ctx(), &awss3.GetBucketPolicyStatusInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get bucket policy status")
	s3util.EqualNow(e.T, out.PolicyStatus != nil, true, "policy status present")
	s3util.Equal(e.T, aws.ToBool(out.PolicyStatus.IsPublic), false, "is public")
}
