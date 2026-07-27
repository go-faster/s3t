package s3

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func bucketTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_list_empty", bucketListEmpty),
		b.add("bucket_list_distinct", bucketListDistinct, "list_objects_v2"),
		b.add("bucket_create_delete", bucketCreateDelete),
		b.add("bucket_delete_notexist", bucketDeleteNotexist),
		b.add("bucket_delete_nonempty", bucketDeleteNonempty),
		b.add("bucket_head", bucketHead),
		b.add("bucket_head_notexist", bucketHeadNotexist),
		b.add("bucket_notexist", bucketNotexist),
	}
}

func bucketListEmpty(e *fixture.Env) {
	bucket := e.NewBucket()

	out, err := e.Client().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list objects")
	s3util.Equal(e.T, len(out.Contents), 0, "object count")
}

// bucketListDistinct checks that an object written to one bucket does not
// appear in another.
func bucketListDistinct(e *fixture.Env) {
	bucket1 := e.NewBucket()
	bucket2 := e.NewBucket()

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket1),
		Key:    aws.String("asdf"),
		Body:   strings.NewReader("str"),
	})
	s3util.NoError(e.T, err, "put object")

	out, err := e.Client().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket2),
	})
	s3util.NoError(e.T, err, "list objects")
	s3util.Equal(e.T, len(out.Contents), 0, "object count")
}

func bucketCreateDelete(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "delete bucket")

	_, err = e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func bucketDeleteNotexist(e *fixture.Env) {
	// Deliberately not created.
	bucket := e.NewBucketName()

	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func bucketDeleteNonempty(e *fixture.Env) {
	bucket := createObjects(e, "foo")

	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 409, "BucketNotEmpty")
}

func bucketHead(e *fixture.Env) {
	bucket := e.NewBucket()

	out, err := e.Client().HeadBucket(e.Ctx(), &awss3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "head bucket")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}

func bucketHeadNotexist(e *fixture.Env) {
	bucket := e.NewBucketName()

	_, err := e.Client().HeadBucket(e.Ctx(), &awss3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	// Only the status is checked: HEAD has no response body to carry an
	// error code, and upstream leaves the code assertion commented out for
	// exactly that reason.
	s3util.ErrorStatus(e.T, err, 404)
}

func bucketNotexist(e *fixture.Env) {
	bucket := e.NewBucketName()

	_, err := e.Client().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}
