package s3

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerConditionalWrite is upstream's marker for the conditional-write tests.
const markerConditionalWrite = "conditional_write"

func versionedTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("versioning_obj_plain_null_version_removal", versioningObjPlainNullVersionRemoval, markerVersioning),
		b.add("versioning_bucket_atomic_upload_return_version_id", versioningBucketAtomicUploadReturnVersionID, markerVersioning),
		b.add("versioning_obj_list_marker", versioningObjListMarker, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_copy_obj_version", versioningCopyObjVersion, markerVersioning, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_versioned_bucket", objectCopyVersionedBucket, markerCopy, "fails_on_dbstore"),
		b.add("object_copy_versioned_url_encoding", objectCopyVersionedURLEncoding, markerCopy, "fails_on_dbstore"),
		b.add("multipart_copy_versioned", multipartCopyVersioned, markerCopy, "fails_on_dbstore"),
		b.add("put_current_object_if_match", putCurrentObjectIfMatch, markerConditionalWrite, "fails_on_dbstore"),
		b.add("put_current_object_if_none_match", putCurrentObjectIfNoneMatch, markerConditionalWrite, "fails_on_dbstore"),
		b.add("multipart_put_current_object_if_none_match", multipartPutCurrentObjectIfNoneMatch, markerConditionalWrite, "fails_on_dbstore"),
	}
}

func versioningObjPlainNullVersionRemoval(e *fixture.Env) {
	bucket := e.NewBucket()
	checkVersioningUnset(e, bucket)

	const key = "testobjfoo"
	putObject(e, bucket, key, "fooz")

	// Enabling versioning leaves the pre-existing object as the "null"
	// version, which can then be deleted by name.
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	deleteVersion(e, bucket, key, "null")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count")
}

func versioningBucketAtomicUploadReturnVersionID(e *fixture.Env) {
	// Versioning enabled: a write returns a version id, and it is the one
	// the listing reports.
	enabled := e.NewBucket()
	configureVersioningRetry(e, enabled, types.BucketVersioningStatusEnabled)

	out := putObject(e, enabled, "bar", "")
	versionID := aws.ToString(out.VersionId)
	s3util.Equal(e.T, versionID != "", true, "version id is set")

	for _, v := range listVersions(e, enabled).Versions {
		s3util.Equal(e.T, aws.ToString(v.VersionId), versionID, "listed version id")
	}

	// Never configured, and suspended: no version id at all.
	s3util.Equal(e.T, aws.ToString(putObject(e, e.NewBucket(), "baz", "").VersionId), "",
		"version id on an unversioned bucket")

	suspended := e.NewBucket()
	configureVersioningRetry(e, suspended, types.BucketVersioningStatusSuspended)
	s3util.Equal(e.T, aws.ToString(putObject(e, suspended, "baz", "").VersionId), "",
		"version id on a suspended bucket")
}

func versioningObjListMarker(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)

	const key = "testobj"
	const key2 = "testobj-1"
	const numVersions = 5

	versionIDs, contents := createMultipleVersions(e, bucket, key, numVersions)
	versionIDs2, contents2 := createMultipleVersions(e, bucket, key2, numVersions)

	// Listings come back newest first; reversed, they run oldest first per
	// key, with the later key's versions ahead of the earlier key's.
	versions := slices.Clone(listVersions(e, bucket).Versions)
	slices.Reverse(versions)
	s3util.EqualNow(e.T, len(versions), 2*numVersions, "version count")

	for i := range numVersions {
		v := versions[i]
		s3util.Equal(e.T, aws.ToString(v.VersionId), versionIDs2[i], "version id of "+key2)
		s3util.Equal(e.T, aws.ToString(v.Key), key2, "key")
		s3util.Equal(e.T, getVersion(e, bucket, key2, versionIDs2[i]), contents2[i], "content")
	}
	for j := range numVersions {
		v := versions[numVersions+j]
		s3util.Equal(e.T, aws.ToString(v.VersionId), versionIDs[j], "version id of "+key)
		s3util.Equal(e.T, aws.ToString(v.Key), key, "key")
		s3util.Equal(e.T, getVersion(e, bucket, key, versionIDs[j]), contents[j], "content")
	}
}

// copySourceVersion builds a CopySource naming a specific version.
func copySourceVersion(bucket, key, versionID string) *string {
	return aws.String(aws.ToString(copySource(bucket, key)) + "?versionId=" + versionID)
}

func versioningCopyObjVersion(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)

	const key = "testobj"
	const numVersions = 3
	versionIDs, contents := createMultipleVersions(e, bucket, key, numVersions)

	// Each version can be copied out to its own key.
	for i := range numVersions {
		newKey := fmt.Sprintf("key_%d", i)
		copyObject(e, &awss3.CopyObjectInput{
			Bucket:     aws.String(bucket),
			CopySource: copySourceVersion(bucket, key, versionIDs[i]),
			Key:        aws.String(newKey),
		})
		s3util.Equal(e.T, getObjectBody(e, bucket, newKey), contents[i], "copy of version "+versionIDs[i])
	}

	// Copying the current version without naming one gives the newest.
	const another = "another"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(bucket, key),
		Key:        aws.String(another),
	})
	s3util.Equal(e.T, getObjectBody(e, bucket, another), contents[numVersions-1], "copy of the current version")
}

func objectCopyVersionedBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)

	const size = 5
	data := strings.Repeat("\x00", size)
	const key1 = "foo123bar"
	putObject(e, bucket, key1, data)

	first, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key1),
	})
	s3util.NoError(e.T, err, "get object")
	versionID := aws.ToString(first.VersionId)
	_ = first.Body.Close()

	// Same bucket.
	const key2 = "bar321foo"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySourceVersion(bucket, key1, versionID),
		Key:        aws.String(key2),
	})
	versionID2 := checkCopiedVersion(e, bucket, key2, data)

	// A copy of the copy, named by its own version.
	const key3 = "bar321foo2"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySourceVersion(bucket, key2, versionID2),
		Key:        aws.String(key3),
	})
	checkCopiedVersion(e, bucket, key3, data)

	// To another versioned bucket.
	versioned2 := e.NewBucket()
	configureVersioningRetry(e, versioned2, types.BucketVersioningStatusEnabled)
	const key4 = "bar321foo3"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(versioned2),
		CopySource: copySourceVersion(bucket, key1, versionID),
		Key:        aws.String(key4),
	})
	checkCopiedVersion(e, versioned2, key4, data)

	// To an unversioned bucket, and back from it.
	plain := e.NewBucket()
	const key5 = "bar321foo4"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(plain),
		CopySource: copySourceVersion(bucket, key1, versionID),
		Key:        aws.String(key5),
	})
	checkCopiedVersion(e, plain, key5, data)

	const key6 = "foo123bar2"
	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySource(plain, key5),
		Key:        aws.String(key6),
	})
	checkCopiedVersion(e, bucket, key6, data)
}

// checkCopiedVersion reads a copy back and returns its version id.
func checkCopiedVersion(e *fixture.Env, bucket, key, data string) string {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "get object "+key)
	defer func() { _ = out.Body.Close() }()

	s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(len(data)), "content length of "+key)
	s3util.Equal(e.T, readAll(e, out.Body) == data, true, "body of "+key)
	return aws.ToString(out.VersionId)
}

func objectCopyVersionedURLEncoding(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)

	// Both keys need escaping in a copy source: '?' would start the query
	// string, '&' would separate parameters.
	const srcKey = "foo?bar"
	const dstKey = "bar&foo"

	put := putObject(e, bucket, srcKey, "")
	_, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(srcKey),
	})
	s3util.NoError(e.T, err, "head source object")

	copyObject(e, &awss3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: copySourceVersion(bucket, srcKey, aws.ToString(put.VersionId)),
		Key:        aws.String(dstKey),
	})

	_, err = e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(dstKey),
	})
	s3util.NoError(e.T, err, "head copied object")
}

func multipartCopyVersioned(e *fixture.Env) {
	srcBucket := e.NewBucket()
	destBucket := e.NewBucket()
	const destKey = "mymultipart"
	const srcKey = "foo"

	checkVersioningUnset(e, srcBucket)
	configureVersioningRetry(e, srcBucket, types.BucketVersioningStatusEnabled)

	const size = 15 * 1024 * 1024
	for range 3 {
		createKeyWithRandomContent(e, srcKey, size, srcBucket)
	}

	var versionIDs []string
	for _, v := range listVersions(e, srcBucket).Versions {
		versionIDs = append(versionIDs, aws.ToString(v.VersionId))
	}
	s3util.EqualNow(e.T, len(versionIDs), 3, "version count")

	// Every version can be the source of a multipart copy.
	for _, vid := range versionIDs {
		uploadID, parts := multipartCopyFromVersion(e, srcBucket, srcKey, destBucket, destKey, size, vid)
		completeMultipart(e, destBucket, destKey, uploadID, parts)

		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(destBucket),
			Key:    aws.String(destKey),
		})
		s3util.NoError(e.T, err, "get object")
		s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
		_ = out.Body.Close()
	}
}

// multipartCopyFromVersion is multipartCopy with a versioned source.
func multipartCopyFromVersion(e *fixture.Env, srcBucket, srcKey, destBucket, destKey string,
	size int, versionID string,
) (uploadID string, parts []types.CompletedPart) {
	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID = aws.ToString(created.UploadId)

	for i, start := 0, 0; start < size; i, start = i+1, start+defaultPartSize {
		end := min(start+defaultPartSize-1, size-1)
		partNum := int32(i + 1)

		out, err := e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
			Bucket:          aws.String(destBucket),
			Key:             aws.String(destKey),
			CopySource:      copySourceVersion(srcBucket, srcKey, versionID),
			CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
			PartNumber:      aws.Int32(partNum),
			UploadId:        aws.String(uploadID),
		})
		s3util.NoError(e.T, err, fmt.Sprintf("upload part copy %d", partNum))
		parts = append(parts, types.CompletedPart{
			ETag:       out.CopyPartResult.ETag,
			PartNumber: aws.Int32(partNum),
		})
	}
	return uploadID, parts
}

// currentObjectKey is the key the conditional-write tests use, as upstream does.
const currentObjectKey = "obj"

// putConditional writes with a conditional header and returns the output and
// error, so callers assert either way.
func putConditional(e *fixture.Env, bucket, body string, in *awss3.PutObjectInput) (*awss3.PutObjectOutput, error) {
	in.Bucket = aws.String(bucket)
	in.Key = aws.String(currentObjectKey)
	if body != "" {
		in.Body = readerOf(body)
	}
	return e.Client().PutObject(e.Ctx(), in)
}

func putCurrentObjectIfMatch(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	const key = currentObjectKey

	// If-Match against a key that does not exist yet is NoSuchKey, not a
	// precondition failure.
	for _, cond := range []string{"*", "badetag"} {
		_, err := putConditional(e, bucket, "", &awss3.PutObjectInput{IfMatch: aws.String(cond)})
		s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
	}

	first, err := putConditional(e, bucket, "data1", &awss3.PutObjectInput{IfNoneMatch: aws.String("deadbeef")})
	s3util.NoError(e.T, err, "put with if-none-match")
	etag := aws.ToString(first.ETag)

	second, err := putConditional(e, bucket, "data2", &awss3.PutObjectInput{IfMatch: aws.String("*")})
	s3util.NoError(e.T, err, "put with if-match *")
	etag2 := aws.ToString(second.ETag)

	// Only the current version's etag satisfies If-Match; the older one no
	// longer does.
	for _, cond := range []string{"badetag", etag} {
		_, err := putConditional(e, bucket, "", &awss3.PutObjectInput{IfMatch: aws.String(cond)})
		s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	}

	_, err = putConditional(e, bucket, "", &awss3.PutObjectInput{IfMatch: aws.String(etag2)})
	s3util.NoError(e.T, err, "put with the current etag")
}

func putCurrentObjectIfNoneMatch(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	const key = currentObjectKey

	first, err := putConditional(e, bucket, "data1", &awss3.PutObjectInput{IfNoneMatch: aws.String("*")})
	s3util.NoError(e.T, err, "put with if-none-match *")
	etag := aws.ToString(first.ETag)

	second := putObject(e, bucket, key, "data2")
	etag2 := aws.ToString(second.ETag)

	// "*" and the current etag both fail once the object exists.
	for _, cond := range []string{"*", etag2} {
		_, err := putConditional(e, bucket, "", &awss3.PutObjectInput{IfNoneMatch: aws.String(cond)})
		s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	}

	// The condition is against the current version only, so an older etag
	// or an unrelated one succeeds.
	for _, cond := range []string{etag, "badetag"} {
		_, err := putConditional(e, bucket, "", &awss3.PutObjectInput{IfNoneMatch: aws.String(cond)})
		s3util.NoError(e.T, err, "put with if-none-match "+cond)
	}

	deleteCurrentObject(e, bucket, key)
}

// conditionalMultipart runs a one-part upload and completes it with an
// optional condition, mirroring upstream's conditional multipart helpers.
func conditionalMultipart(e *fixture.Env, bucket, body, ifNoneMatch string) (
	*awss3.CompleteMultipartUploadOutput, error,
) {
	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(currentObjectKey),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	part, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(currentObjectKey),
		UploadId:   created.UploadId,
		PartNumber: aws.Int32(1),
		Body:       readerOf(body),
	})
	s3util.NoError(e.T, err, "upload part")

	in := &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(currentObjectKey),
		UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{{ETag: part.ETag, PartNumber: aws.Int32(1)}},
		},
	}
	if ifNoneMatch != "" {
		in.IfNoneMatch = aws.String(ifNoneMatch)
	}
	return e.Client().CompleteMultipartUpload(e.Ctx(), in)
}

func multipartPutCurrentObjectIfNoneMatch(e *fixture.Env) {
	bucket := e.NewBucket()
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	const key = currentObjectKey

	first, err := conditionalMultipart(e, bucket, "data1", "*")
	s3util.NoError(e.T, err, "complete with if-none-match *")
	etag := aws.ToString(first.ETag)

	second, err := conditionalMultipart(e, bucket, "data2", "")
	s3util.NoError(e.T, err, "complete without a condition")
	etag2 := aws.ToString(second.ETag)

	for _, cond := range []string{"*", etag2} {
		_, err := conditionalMultipart(e, bucket, "abc", cond)
		s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")
	}

	// As with the single-part case, the condition applies to the current
	// version only.
	for _, cond := range []string{etag, "badetag"} {
		_, err := conditionalMultipart(e, bucket, "abc", cond)
		s3util.NoError(e.T, err, "complete with if-none-match "+cond)
	}

	deleteCurrentObject(e, bucket, key)
}

// deleteCurrentObject removes the current version of a key.
func deleteCurrentObject(e *fixture.Env, bucket, key string) {
	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "delete object")
}
