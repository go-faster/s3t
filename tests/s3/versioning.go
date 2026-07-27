package s3

import (
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerVersioning is on every test in this file, matching upstream.
const markerVersioning = "versioning"

func versioningTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("versioning_bucket_create_suspend", versioningBucketCreateSuspend, markerVersioning),
		b.add("versioning_obj_create_read_remove", versioningObjCreateReadRemove, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_obj_create_read_remove_head", versioningObjCreateReadRemoveHead, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_obj_plain_null_version_overwrite", versioningObjPlainNullVersionOverwrite, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_obj_plain_null_version_overwrite_suspended", versioningObjPlainNullVersionOverwriteSuspended, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_obj_create_versions_remove_all", versioningObjCreateVersionsRemoveAll, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_obj_create_versions_remove_special_names", versioningObjCreateVersionsRemoveSpecialNames, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_multi_object_delete", versioningMultiObjectDelete, markerVersioning, "fails_on_dbstore"),
		b.add("versioning_multi_object_delete_with_marker", versioningMultiObjectDeleteWithMarker, markerVersioning, "fails_on_dbstore"),
	}
}

// setVersioning sets a bucket's versioning status.
func setVersioning(e *fixture.Env, bucket string, status types.BucketVersioningStatus) {
	_, err := e.Client().PutBucketVersioning(e.Ctx(), &awss3.PutBucketVersioningInput{
		Bucket:                  aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{Status: status},
	})
	s3util.NoError(e.T, err, "put bucket versioning")
}

// checkVersioningUnset asserts the bucket has never had versioning configured,
// which S3 reports as no status at all.
func checkVersioningUnset(e *fixture.Env, bucket string) {
	out, err := e.Client().GetBucketVersioning(e.Ctx(), &awss3.GetBucketVersioningInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get bucket versioning")
	s3util.Equal(e.T, out.Status, types.BucketVersioningStatus(""), "versioning status")
}

// configureVersioningRetry sets the status and waits for it to read back,
// mirroring upstream's check_configure_versioning_retry. The retry is there
// because the setting may propagate asynchronously.
func configureVersioningRetry(e *fixture.Env, bucket string, status types.BucketVersioningStatus) {
	setVersioning(e, bucket, status)

	for range 5 {
		out, err := e.Client().GetBucketVersioning(e.Ctx(), &awss3.GetBucketVersioningInput{
			Bucket: aws.String(bucket),
		})
		s3util.NoError(e.T, err, "get bucket versioning")
		if out.Status == status {
			return
		}
		time.Sleep(time.Second)
	}
	e.T.Errorf("versioning status did not become %s", status)
}

// newVersionedBucket creates a bucket with versioning enabled.
func newVersionedBucket(e *fixture.Env) string {
	bucket := e.NewBucket()
	setVersioning(e, bucket, types.BucketVersioningStatusEnabled)
	return bucket
}

// createMultipleVersions writes num versions of a key and returns their ids and
// contents, oldest first.
func createMultipleVersions(e *fixture.Env, bucket, key string, num int) (versionIDs, contents []string) {
	for i := range num {
		body := fmt.Sprintf("content-%d", i)
		out, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   readerOf(body),
		})
		s3util.NoError(e.T, err, "put object")
		contents = append(contents, body)
		versionIDs = append(versionIDs, aws.ToString(out.VersionId))
	}
	return versionIDs, contents
}

// getVersion reads one version of a key.
func getVersion(e *fixture.Env, bucket, key, versionID string) string {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(key),
		VersionId: aws.String(versionID),
	})
	s3util.NoError(e.T, err, "get object version")
	defer func() { _ = out.Body.Close() }()
	return readAll(e, out.Body)
}

// deleteVersion removes one version of a key.
func deleteVersion(e *fixture.Env, bucket, key, versionID string) {
	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(key),
		VersionId: aws.String(versionID),
	})
	s3util.NoError(e.T, err, "delete object version")
}

// listVersions returns a bucket's object versions and delete markers.
func listVersions(e *fixture.Env, bucket string) *awss3.ListObjectVersionsOutput {
	out, err := e.Client().ListObjectVersions(e.Ctx(), &awss3.ListObjectVersionsInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list object versions")
	return out
}

// checkObjVersions asserts the listing matches the expected ids and contents,
// mirroring upstream's check_obj_versions. Listings come back newest first, so
// they are reversed to compare against creation order.
func checkObjVersions(e *fixture.Env, bucket, key string, versionIDs, contents []string) {
	out := listVersions(e, bucket)
	versions := slices.Clone(out.Versions)
	slices.Reverse(versions)

	if len(versions) != len(versionIDs) {
		e.T.Errorf("listing has %d versions, want %d", len(versions), len(versionIDs))
		return
	}
	for i, v := range versions {
		s3util.Equal(e.T, aws.ToString(v.VersionId), versionIDs[i], "version id")
		s3util.Equal(e.T, aws.ToString(v.Key), key, "key")
		s3util.Equal(e.T, getVersion(e, bucket, key, versionIDs[i]), contents[i], "version content")
	}
}

// removeObjVersion deletes the version at index and checks what is left,
// mirroring upstream's remove_obj_version. A negative index counts from the
// end, as Python's list indexing does.
func removeObjVersion(e *fixture.Env, bucket, key string, versionIDs, contents *[]string, index int) {
	n := len(*versionIDs)
	if n == 0 {
		e.T.Fatalf("no versions left to remove")
	}
	i := ((index % n) + n) % n

	rmID, rmContent := (*versionIDs)[i], (*contents)[i]
	s3util.Equal(e.T, getVersion(e, bucket, key, rmID), rmContent, "version content before removal")

	*versionIDs = slices.Delete(slices.Clone(*versionIDs), i, i+1)
	*contents = slices.Delete(slices.Clone(*contents), i, i+1)

	deleteVersion(e, bucket, key, rmID)

	if len(*versionIDs) != 0 {
		checkObjVersions(e, bucket, key, *versionIDs, *contents)
	}
}

// doTestCreateRemoveVersions writes num versions then removes them one at a
// time, starting at removeStartIdx and stepping by idxInc.
func doTestCreateRemoveVersions(e *fixture.Env, bucket, key string, num, removeStartIdx, idxInc int) {
	versionIDs, contents := createMultipleVersions(e, bucket, key, num)

	idx := removeStartIdx
	for range num {
		removeObjVersion(e, bucket, key, &versionIDs, &contents, idx)
		idx += idxInc
	}
}

func versioningBucketCreateSuspend(e *fixture.Env) {
	bucket := e.NewBucket()
	// A bucket that has never been configured reports no status at all.
	checkVersioningUnset(e, bucket)

	for _, status := range []types.BucketVersioningStatus{
		types.BucketVersioningStatusSuspended,
		types.BucketVersioningStatusEnabled,
		types.BucketVersioningStatusEnabled,
		types.BucketVersioningStatusSuspended,
	} {
		configureVersioningRetry(e, bucket, status)
	}
}

func versioningObjCreateReadRemove(e *fixture.Env) {
	bucket := e.NewBucket()
	_, err := e.Client().PutBucketVersioning(e.Ctx(), &awss3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{
			MFADelete: types.MFADeleteDisabled,
			Status:    types.BucketVersioningStatusEnabled,
		},
	})
	s3util.NoError(e.T, err, "put bucket versioning")

	const key = "testobj"
	const numVersions = 5

	// Remove in several orders: newest first, oldest first, and walking
	// forwards and backwards through the list.
	for _, order := range []struct{ start, inc int }{
		{-1, 0}, {-1, 0}, {0, 0}, {1, 0}, {4, -1}, {3, 3},
	} {
		doTestCreateRemoveVersions(e, bucket, key, numVersions, order.start, order.inc)
	}
}

func versioningObjCreateReadRemoveHead(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	const key = "testobj"
	numVersions := 5

	versionIDs, contents := createMultipleVersions(e, bucket, key, numVersions)

	// Removing the head must expose the version under it.
	removed := versionIDs[len(versionIDs)-1]
	contents = contents[:len(contents)-1]
	numVersions--

	deleteVersion(e, bucket, key, removed)
	s3util.Equal(e.T, getObjectBody(e, bucket, key), contents[len(contents)-1], "body after removing head")

	// Deleting without a version id adds a delete marker instead.
	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "delete object")
	s3util.Equal(e.T, aws.ToBool(out.DeleteMarker), true, "delete marker")

	listed := listVersions(e, bucket)
	s3util.Equal(e.T, len(listed.Versions), numVersions, "version count")
	s3util.EqualNow(e.T, len(listed.DeleteMarkers), 1, "delete marker count")
	s3util.Equal(e.T, aws.ToString(listed.DeleteMarkers[0].VersionId),
		aws.ToString(out.VersionId), "delete marker version id")
}

func versioningObjPlainNullVersionOverwrite(e *fixture.Env) {
	bucket := e.NewBucket()
	checkVersioningUnset(e, bucket)

	const key = "testobjfoo"
	const content = "fooz"
	putObject(e, bucket, key, content)

	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)

	// The pre-versioning object keeps the "null" version id.
	const content2 = "zzz"
	out := putObject(e, bucket, key, content2)
	s3util.Equal(e.T, getObjectBody(e, bucket, key), content2, "body")

	deleteVersion(e, bucket, key, aws.ToString(out.VersionId))
	s3util.Equal(e.T, getObjectBody(e, bucket, key), content, "body after removing the new version")

	deleteVersion(e, bucket, key, "null")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count")
}

func versioningObjPlainNullVersionOverwriteSuspended(e *fixture.Env) {
	bucket := e.NewBucket()
	checkVersioningUnset(e, bucket)

	const key = "testobjbar"
	const content = "foooz"
	putObject(e, bucket, key, content)

	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusSuspended)

	// While suspended, a write replaces the null version rather than adding
	// one.
	const content2 = "zzz"
	putObject(e, bucket, key, content2)
	s3util.Equal(e.T, getObjectBody(e, bucket, key), content2, "body")

	listed := listVersions(e, bucket)
	s3util.EqualNow(e.T, len(listed.Versions), 1, "version count")
	s3util.Equal(e.T, aws.ToString(listed.Versions[0].VersionId), "null", "version id")

	deleteVersion(e, bucket, key, "null")

	_, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count")
}

func versioningObjCreateVersionsRemoveAll(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	const key = "testobj"
	const numVersions = 10

	versionIDs, contents := createMultipleVersions(e, bucket, key, numVersions)
	for range numVersions {
		removeObjVersion(e, bucket, key, &versionIDs, &contents, 0)
	}
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count")
}

func versioningObjCreateVersionsRemoveSpecialNames(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	const numVersions = 10

	for _, key := range []string{"_testobj", "_", ":", " "} {
		versionIDs, contents := createMultipleVersions(e, bucket, key, numVersions)
		for range numVersions {
			removeObjVersion(e, bucket, key, &versionIDs, &contents, 0)
		}
		s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count for "+key)
	}
}

func versioningMultiObjectDelete(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	const key = "key"

	createMultipleVersions(e, bucket, key, 2)
	listed := listVersions(e, bucket)
	s3util.Equal(e.T, len(listed.Versions), 2, "version count")

	// Deleting every version by id leaves nothing, not even a marker.
	deleteAllVersions(e, bucket, listed)
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count after delete")

	// Repeating the delete is not an error.
	deleteAllVersions(e, bucket, listed)
	s3util.Equal(e.T, len(listVersions(e, bucket).Versions), 0, "version count after second delete")
}

func versioningMultiObjectDeleteWithMarker(e *fixture.Env) {
	bucket := newVersionedBucket(e)
	const key = "key"

	createMultipleVersions(e, bucket, key, 1)

	// A keyed delete adds a marker, which is itself a version to remove.
	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "delete object")
	s3util.Equal(e.T, aws.ToBool(out.DeleteMarker), true, "delete marker")

	listed := listVersions(e, bucket)
	s3util.Equal(e.T, len(listed.Versions), 1, "version count")
	s3util.Equal(e.T, len(listed.DeleteMarkers), 1, "delete marker count")

	deleteAllVersions(e, bucket, listed)

	after := listVersions(e, bucket)
	s3util.Equal(e.T, len(after.Versions), 0, "version count after delete")
	s3util.Equal(e.T, len(after.DeleteMarkers), 0, "delete marker count after delete")
}

// deleteAllVersions removes every version and delete marker in a listing.
func deleteAllVersions(e *fixture.Env, bucket string, listed *awss3.ListObjectVersionsOutput) {
	ids := make([]types.ObjectIdentifier, 0, len(listed.Versions)+len(listed.DeleteMarkers))
	for _, v := range listed.Versions {
		ids = append(ids, types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
	}
	for _, m := range listed.DeleteMarkers {
		ids = append(ids, types.ObjectIdentifier{Key: m.Key, VersionId: m.VersionId})
	}
	if len(ids) == 0 {
		return
	}

	out, err := e.Client().DeleteObjects(e.Ctx(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: ids},
	})
	s3util.NoError(e.T, err, "delete objects")
	s3util.Equal(e.T, len(out.Errors), 0, "delete error count")
}
