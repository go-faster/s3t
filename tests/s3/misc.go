package s3

import (
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func miscTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("abort_multipart_upload", abortMultipartUpload),
		b.add("abort_multipart_upload_not_found", abortMultipartUploadNotFound),
		b.add("basic_key_count", basicKeyCount, markerV2),
		b.add("buckets_create_then_list", bucketsCreateThenList),
		b.add("buckets_list_ctime", bucketsListCtime),
		b.add("bucketv2_notexist", bucketv2Notexist, markerV2),
		b.add("list_buckets_anonymous", listBucketsAnonymous, harness.MarkerSerial, "fails_on_aws"),
		b.add("list_buckets_bad_auth", listBucketsBadAuth),
		b.add("list_buckets_paginated", listBucketsPaginated, harness.MarkerSerial, "fails_on_dbstore"),
		b.add("list_buckets_invalid_auth", listBucketsInvalidAuth),
	}
}

func abortMultipartUpload(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	uploadID, _, _ := multipartUpload(e, bucket, key, 10*1024*1024, multipartOpts{})

	_, err := e.Client().AbortMultipartUpload(e.Ctx(), &awss3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	s3util.NoError(e.T, err, "abort multipart upload")

	// An aborted upload leaves no object behind.
	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(key),
	})
	s3util.Equal(e.T, len(out.Contents), 0, "object count")
}

func abortMultipartUploadNotFound(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"
	putObject(e, bucket, key, "")

	_, err := e.Client().AbortMultipartUpload(e.Ctx(), &awss3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String("56788"),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchUpload")
}

func basicKeyCount(e *fixture.Env) {
	bucket := e.NewBucket()
	for j := range 5 {
		putObject(e, bucket, strconv.Itoa(j), "")
	}

	out := listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, aws.ToInt32(out.KeyCount), 5, "key count")
}

// listOwnBuckets returns the buckets belonging to this test, filtered by its
// prefix. Upstream filters by the global prefix for the same reason: a shared
// server holds other people's buckets too.
func listOwnBuckets(e *fixture.Env) []string {
	out, err := e.Client().ListBuckets(e.Ctx(), &awss3.ListBucketsInput{})
	s3util.NoError(e.T, err, "list buckets")

	var names []string
	for _, b := range out.Buckets {
		if name := aws.ToString(b.Name); strings.HasPrefix(name, e.Prefix()) {
			names = append(names, name)
		}
	}
	return names
}

func bucketsCreateThenList(e *fixture.Env) {
	created := make([]string, 0, 5)
	for range 5 {
		created = append(created, e.NewBucket())
	}

	listed := listOwnBuckets(e)
	for _, name := range created {
		found := false
		for _, got := range listed {
			if got == name {
				found = true
				break
			}
		}
		s3util.Equal(e.T, found, true, "bucket "+name+" is listed")
	}
}

func bucketsListCtime(e *fixture.Env) {
	// Creation times must be recent; a day of slack absorbs clock skew.
	before := time.Now().UTC().Add(-24 * time.Hour)

	created := map[string]bool{}
	for range 5 {
		created[e.NewBucket()] = true
	}

	out, err := e.Client().ListBuckets(e.Ctx(), &awss3.ListBucketsInput{})
	s3util.NoError(e.T, err, "list buckets")

	for _, b := range out.Buckets {
		if !created[aws.ToString(b.Name)] {
			continue
		}
		s3util.Equal(e.T, !aws.ToTime(b.CreationDate).Before(before), true,
			"creation date of "+aws.ToString(b.Name)+" is recent")
	}
}

func bucketv2Notexist(e *fixture.Env) {
	bucket := e.NewBucketName()

	_, err := e.Client().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func listBucketsBadAuth(e *fixture.Env) {
	// A real access key with the wrong secret.
	_, err := e.BadAuthClient(e.Cfg.Main.AccessKey).ListBuckets(e.Ctx(), &awss3.ListBucketsInput{})
	s3util.ErrorStatus(e.T, err, 403)
}

func listBucketsInvalidAuth(e *fixture.Env) {
	// An access key that does not exist at all.
	_, err := e.BadAuthClient("badauth").ListBuckets(e.Ctx(), &awss3.ListBucketsInput{})
	s3util.ErrorStatus(e.T, err, 403)
}

// listBucketsAnonymous and listBucketsPaginated count every bucket in the
// account, so they cannot run beside tests that are creating and deleting
// their own. The serial marker is not an upstream one; upstream runs the whole
// suite serially and does not need it.
func listBucketsAnonymous(e *fixture.Env) {
	out, err := e.AnonymousClient().ListBuckets(e.Ctx(), &awss3.ListBucketsInput{})
	s3util.NoError(e.T, err, "list buckets anonymously")
	s3util.Equal(e.T, len(out.Buckets), 0, "bucket count")
}

func listBucketsPaginated(e *fixture.Env) {
	list := func(token string) *awss3.ListBucketsOutput {
		in := &awss3.ListBucketsInput{MaxBuckets: aws.Int32(1)}
		if token != "" {
			in.ContinuationToken = aws.String(token)
		}
		out, err := e.Client().ListBuckets(e.Ctx(), in)
		s3util.NoError(e.T, err, "list buckets")
		return out
	}

	out := list("")
	s3util.Equal(e.T, len(out.Buckets), 0, "bucket count with none created")
	s3util.Equal(e.T, out.ContinuationToken == nil, true, "continuation token absent")

	bucket1 := e.NewBucket()
	out = list("")
	s3util.EqualNow(e.T, len(out.Buckets), 1, "bucket count after one")
	s3util.Equal(e.T, aws.ToString(out.Buckets[0].Name), bucket1, "first bucket")
	s3util.Equal(e.T, out.ContinuationToken == nil, true, "continuation token absent")

	bucket2 := e.NewBucket()
	out = list("")
	s3util.EqualNow(e.T, len(out.Buckets), 1, "page size")
	s3util.Equal(e.T, aws.ToString(out.Buckets[0].Name), bucket1, "first page")
	token := mustField(e, out.ContinuationToken, "ContinuationToken")

	out = list(token)
	s3util.EqualNow(e.T, len(out.Buckets), 1, "page size")
	s3util.Equal(e.T, aws.ToString(out.Buckets[0].Name), bucket2, "second page")
	s3util.Equal(e.T, out.ContinuationToken == nil, true, "continuation token absent")
}
