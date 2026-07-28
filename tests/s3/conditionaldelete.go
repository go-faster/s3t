package s3

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// The conditional deletes are only implemented for directory buckets on AWS,
// so every test here carries fails_on_aws upstream.
const markerFailsOnAWS = "fails_on_aws"

func conditionalDeleteTests(b builder) []harness.Test {
	m := []string{markerFailsOnAWS, markerConditionalWrite, markerDBStore}
	return []harness.Test{
		b.add("delete_object_current_if_match", deleteObjectCurrentIfMatch, m...),
		b.add("delete_object_current_if_match_last_modified_time", deleteObjectCurrentIfMatchLastModifiedTime, m...),
		b.add("delete_object_current_if_match_size", deleteObjectCurrentIfMatchSize, m...),
		b.add("delete_object_if_match", deleteObjectIfMatch, m...),
		b.add("delete_object_if_match_last_modified_time", deleteObjectIfMatchLastModifiedTime, m...),
		b.add("delete_object_if_match_size", deleteObjectIfMatchSize, m...),
		b.add("delete_object_version_if_match", deleteObjectVersionIfMatch, m...),
		b.add("delete_object_version_if_match_last_modified_time", deleteObjectVersionIfMatchLastModifiedTime, m...),
		b.add("delete_object_version_if_match_size", deleteObjectVersionIfMatchSize, m...),
		b.add("delete_objects_current_if_match", deleteObjectsCurrentIfMatch, m...),
		b.add("delete_objects_current_if_match_last_modified_time", deleteObjectsCurrentIfMatchLastModifiedTime, m...),
		b.add("delete_objects_current_if_match_size", deleteObjectsCurrentIfMatchSize, m...),
		b.add("delete_objects_if_match", deleteObjectsIfMatch, m...),
		b.add("delete_objects_if_match_last_modified_time", deleteObjectsIfMatchLastModifiedTime, m...),
		b.add("delete_objects_if_match_size", deleteObjectsIfMatchSize, m...),
		b.add("delete_objects_version_if_match", deleteObjectsVersionIfMatch, m...),
		b.add("delete_objects_version_if_match_last_modified_time", deleteObjectsVersionIfMatchLastModifiedTime, m...),
		b.add("delete_objects_version_if_match_size", deleteObjectsVersionIfMatchSize, m...),
	}
}

// The object and the mismatched conditions every test in this file uses, as
// upstream. The object is empty, which is why the size to match is zero.
const (
	condDelKey  = "obj"
	badETag     = "badetag"
	badSize     = 9999
	condDelSize = 0
)

// badMtime is upstream's datetime.datetime(2015, 1, 1).
func badMtime() time.Time { return time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC) }

// deleteCond deletes the object under a condition and returns the result,
// error and all: half of these calls are meant to fail.
func deleteCond(e *fixture.Env, bucket string,
	opt func(*awss3.DeleteObjectInput),
) (*awss3.DeleteObjectOutput, error) {
	in := &awss3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(condDelKey)}
	opt(in)
	return e.Client().DeleteObject(e.Ctx(), in)
}

// deleteCondOK is deleteCond for the calls that have to succeed.
func deleteCondOK(e *fixture.Env, bucket string, opt func(*awss3.DeleteObjectInput)) *awss3.DeleteObjectOutput {
	out, err := deleteCond(e, bucket, opt)
	s3util.NoError(e.T, err, "delete object")
	return out
}

// deleteCondFails asserts the condition was refused.
func deleteCondFails(e *fixture.Env, bucket string, opt func(*awss3.DeleteObjectInput)) {
	_, err := deleteCond(e, bucket, opt)
	s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
}

// deleteCondGone asserts a delete of something that is not there still answers
// 204: a conditional delete of a missing object is not an error.
func deleteCondGone(e *fixture.Env, bucket string, opt func(*awss3.DeleteObjectInput)) {
	out := deleteCondOK(e, bucket, opt)
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")
}

// batchDelete deletes one object through DeleteObjects, which carries the
// condition on the object identifier rather than in a header.
func batchDelete(e *fixture.Env, bucket string, id types.ObjectIdentifier) *awss3.DeleteObjectsOutput {
	id.Key = aws.String(condDelKey)
	out, err := e.Client().DeleteObjects(e.Ctx(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: []types.ObjectIdentifier{id}},
	})
	s3util.NoError(e.T, err, "delete objects")
	return out
}

// batchDeleteFails asserts the batch reported the one object as refused. The
// call itself succeeds; a failed condition is reported per object.
func batchDeleteFails(e *fixture.Env, bucket string, id types.ObjectIdentifier) {
	out := batchDelete(e, bucket, id)
	s3util.EqualNow(e.T, len(out.Errors), 1, "error count")
	s3util.Equal(e.T, aws.ToString(out.Errors[0].Code), "PreconditionFailed", "error code")
}

// batchDeleteOK asserts the batch removed the one object, and returns it.
func batchDeleteOK(e *fixture.Env, bucket string, id types.ObjectIdentifier) types.DeletedObject {
	out := batchDelete(e, bucket, id)
	s3util.EqualNow(e.T, len(out.Deleted), 1, "deleted count")
	s3util.Equal(e.T, aws.ToString(out.Deleted[0].Key), condDelKey, "deleted key")
	return out.Deleted[0]
}

// batchDeleteGone asserts a batch delete of something that is not there still
// answers 200.
func batchDeleteGone(e *fixture.Env, bucket string, id types.ObjectIdentifier) {
	out := batchDelete(e, bucket, id)
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}

// lastModified reads the object's modification time, of a specific version
// when versionID is given.
func lastModified(e *fixture.Env, bucket, versionID string) time.Time {
	in := &awss3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(condDelKey)}
	if versionID != "" {
		in.VersionId = aws.String(versionID)
	}
	out, err := e.Client().HeadObject(e.Ctx(), in)
	s3util.NoError(e.T, err, "head object")
	return aws.ToTime(out.LastModified)
}

func deleteObjectCurrentIfMatch(e *fixture.Env) {
	bucket := newVersionedBucket(e)

	// Deleting what is not there is not an error, whatever the condition.
	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String("*") })
	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(badETag) })

	put := putObject(e, bucket, condDelKey, "")
	version := mustField(e, put.VersionId, "VersionId")
	etag := mustField(e, put.ETag, "ETag")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(badETag) })

	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(etag) })
	s3util.Equal(e.T, aws.ToBool(out.DeleteMarker), true, "delete marker")

	// The delete marker is now current, so the etag condition has nothing to
	// match and "*" still succeeds.
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String("*") })
	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(badETag) })

	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.VersionId = aws.String(version) })
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String("*") })
}

func deleteObjectCurrentIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	putObject(e, bucket, condDelKey, "")
	mtime := lastModified(e, bucket, "")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(badMtime())
	})

	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(mtime)
	})
	s3util.Equal(e.T, aws.ToBool(out.DeleteMarker), true, "delete marker")

	// The object is marked deleted but still there.
	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(badMtime())
	})
}

func deleteObjectCurrentIfMatchSize(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	putObject(e, bucket, condDelKey, "")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(badSize) })

	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(condDelSize) })
	s3util.Equal(e.T, out.DeleteMarker != nil, true, "response carries a delete marker")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(badSize) })
}

func deleteObjectIfMatch(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := mustField(e, putObject(e, bucket, condDelKey, "").ETag, "ETag")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(badETag) })
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(etag) })

	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String("*") })
	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String(badETag) })

	putObject(e, bucket, condDelKey, "")
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatch = aws.String("*") })
}

func deleteObjectIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, condDelKey, "")
	mtime := lastModified(e, bucket, "")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(badMtime())
	})
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(mtime)
	})
	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.IfMatchLastModifiedTime = aws.Time(badMtime())
	})
}

func deleteObjectIfMatchSize(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, condDelKey, "")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(badSize) })
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(condDelSize) })
	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) { in.IfMatchSize = aws.Int64(badSize) })
}

func deleteObjectVersionIfMatch(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	put := putObject(e, bucket, condDelKey, "")
	version := mustField(e, put.VersionId, "VersionId")
	etag := mustField(e, put.ETag, "ETag")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatch = aws.String(version), aws.String(badETag)
	})

	// Deleting a version removes it outright rather than leaving a marker.
	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatch = aws.String(version), aws.String(etag)
	})
	s3util.Equal(e.T, out.DeleteMarker == nil, true, "response carries no delete marker")

	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatch = aws.String(version), aws.String("*")
	})
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatch = aws.String(version), aws.String(badETag)
	})

	version = mustField(e, putObject(e, bucket, condDelKey, "").VersionId, "VersionId")
	deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatch = aws.String(version), aws.String("*")
	})
}

func deleteObjectVersionIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	version := mustField(e, putObject(e, bucket, condDelKey, "").VersionId, "VersionId")
	mtime := lastModified(e, bucket, version)

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchLastModifiedTime = aws.String(version), aws.Time(badMtime())
	})

	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchLastModifiedTime = aws.String(version), aws.Time(mtime)
	})
	s3util.Equal(e.T, out.DeleteMarker == nil, true, "response carries no delete marker")

	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchLastModifiedTime = aws.String(version), aws.Time(badMtime())
	})
}

func deleteObjectVersionIfMatchSize(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	version := mustField(e, putObject(e, bucket, condDelKey, "").VersionId, "VersionId")

	deleteCondFails(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchSize = aws.String(version), aws.Int64(badSize)
	})

	out := deleteCondOK(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchSize = aws.String(version), aws.Int64(condDelSize)
	})
	s3util.Equal(e.T, out.DeleteMarker == nil, true, "response carries no delete marker")

	deleteCondGone(e, bucket, func(in *awss3.DeleteObjectInput) {
		in.VersionId, in.IfMatchSize = aws.String(version), aws.Int64(badSize)
	})
}

func deleteObjectsCurrentIfMatch(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	etag := mustField(e, putObject(e, bucket, condDelKey, "").ETag, "ETag")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{ETag: aws.String(badETag)})

	deleted := batchDeleteOK(e, bucket, types.ObjectIdentifier{ETag: aws.String(etag)})
	s3util.Equal(e.T, aws.ToBool(deleted.DeleteMarker), true, "delete marker")

	// The object is marked deleted but still there.
	batchDeleteFails(e, bucket, types.ObjectIdentifier{ETag: aws.String(badETag)})
}

func deleteObjectsCurrentIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	putObject(e, bucket, condDelKey, "")
	mtime := lastModified(e, bucket, "")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(badMtime())})

	deleted := batchDeleteOK(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(mtime)})
	s3util.Equal(e.T, aws.ToBool(deleted.DeleteMarker), true, "delete marker")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(badMtime())})
}

func deleteObjectsCurrentIfMatchSize(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	putObject(e, bucket, condDelKey, "")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{Size: aws.Int64(badSize)})

	deleted := batchDeleteOK(e, bucket, types.ObjectIdentifier{Size: aws.Int64(condDelSize)})
	s3util.Equal(e.T, aws.ToBool(deleted.DeleteMarker), true, "delete marker")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{Size: aws.Int64(badSize)})
}

func deleteObjectsIfMatch(e *fixture.Env) {
	bucket := e.NewBucket()
	etag := mustField(e, putObject(e, bucket, condDelKey, "").ETag, "ETag")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{ETag: aws.String(badETag)})
	batchDeleteOK(e, bucket, types.ObjectIdentifier{ETag: aws.String(etag)})
	batchDeleteGone(e, bucket, types.ObjectIdentifier{ETag: aws.String(badETag)})
}

func deleteObjectsIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, condDelKey, "")
	mtime := lastModified(e, bucket, "")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(badMtime())})
	batchDeleteOK(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(mtime)})
	batchDeleteGone(e, bucket, types.ObjectIdentifier{LastModifiedTime: aws.Time(badMtime())})
}

func deleteObjectsIfMatchSize(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, condDelKey, "")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{Size: aws.Int64(badSize)})
	batchDeleteOK(e, bucket, types.ObjectIdentifier{Size: aws.Int64(condDelSize)})
	batchDeleteGone(e, bucket, types.ObjectIdentifier{Size: aws.Int64(badSize)})
}

func deleteObjectsVersionIfMatch(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	put := putObject(e, bucket, condDelKey, "")
	version := mustField(e, put.VersionId, "VersionId")
	etag := mustField(e, put.ETag, "ETag")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), ETag: aws.String(badETag),
	})

	deleted := batchDeleteOK(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), ETag: aws.String(etag),
	})
	s3util.Equal(e.T, deleted.DeleteMarker == nil, true, "entry carries no delete marker")

	batchDeleteGone(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), ETag: aws.String(badETag),
	})
}

func deleteObjectsVersionIfMatchLastModifiedTime(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	version := mustField(e, putObject(e, bucket, condDelKey, "").VersionId, "VersionId")
	mtime := lastModified(e, bucket, version)

	// A second version becomes current, so the condition is checked against
	// the older one it names rather than against the head of the key.
	putObject(e, bucket, condDelKey, "")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), LastModifiedTime: aws.Time(badMtime()),
	})
	batchDeleteOK(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), LastModifiedTime: aws.Time(mtime),
	})
	batchDeleteGone(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), LastModifiedTime: aws.Time(badMtime()),
	})
}

func deleteObjectsVersionIfMatchSize(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	version := mustField(e, putObject(e, bucket, condDelKey, "").VersionId, "VersionId")

	batchDeleteFails(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), Size: aws.Int64(badSize),
	})
	batchDeleteOK(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), Size: aws.Int64(condDelSize),
	})
	batchDeleteGone(e, bucket, types.ObjectIdentifier{
		VersionId: aws.String(version), Size: aws.Int64(badSize),
	})
}
