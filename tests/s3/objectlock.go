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

// markerDBStore is on nearly every test in this file, matching upstream.
const markerDBStore = "fails_on_dbstore"

func objectLockTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_lock_put_obj_lock", objectLockPutObjLock, markerDBStore),
		b.add("object_lock_put_obj_lock_invalid_bucket", objectLockPutObjLockInvalidBucket),
		b.add("object_lock_put_obj_lock_enable_after_create", objectLockPutObjLockEnableAfterCreate),
		b.add("object_lock_put_obj_lock_with_days_and_years", objectLockPutObjLockWithDaysAndYears, markerDBStore),
		b.add("object_lock_put_obj_lock_invalid_days", objectLockPutObjLockInvalidDays, markerDBStore),
		b.add("object_lock_put_obj_lock_invalid_years", objectLockPutObjLockInvalidYears, markerDBStore),
		b.add("object_lock_put_obj_lock_invalid_mode", objectLockPutObjLockInvalidMode, markerDBStore),
		b.add("object_lock_put_obj_lock_invalid_status", objectLockPutObjLockInvalidStatus, markerDBStore),
		b.add("object_lock_suspend_versioning", objectLockSuspendVersioning, markerDBStore),
		b.add("object_lock_get_obj_lock", objectLockGetObjLock, markerDBStore),
		b.add("object_lock_get_obj_lock_invalid_bucket", objectLockGetObjLockInvalidBucket),
		b.add("object_lock_put_obj_retention", objectLockPutObjRetention, markerDBStore),
		b.add("object_lock_put_obj_retention_invalid_bucket", objectLockPutObjRetentionInvalidBucket),
		b.add("object_lock_put_obj_retention_invalid_mode", objectLockPutObjRetentionInvalidMode, markerDBStore),
		b.add("object_lock_get_obj_retention", objectLockGetObjRetention, markerDBStore),
		b.add("object_lock_get_obj_retention_iso8601", objectLockGetObjRetentionISO8601, markerDBStore),
		b.add("object_lock_get_obj_retention_invalid_bucket", objectLockGetObjRetentionInvalidBucket),
		b.add("object_lock_put_obj_retention_versionid", objectLockPutObjRetentionVersionID, markerDBStore),
		b.add("object_lock_put_obj_retention_override_default_retention",
			objectLockPutObjRetentionOverrideDefaultRetention, markerDBStore),
		b.add("object_lock_put_obj_retention_increase_period", objectLockPutObjRetentionIncreasePeriod, markerDBStore),
		b.add("object_lock_put_obj_retention_shorten_period", objectLockPutObjRetentionShortenPeriod, markerDBStore),
		b.add("object_lock_put_obj_retention_shorten_period_bypass",
			objectLockPutObjRetentionShortenPeriodBypass, markerDBStore),
		b.add("object_lock_delete_object_with_retention", objectLockDeleteObjectWithRetention, markerDBStore),
		b.add("object_lock_delete_multipart_object_with_retention",
			objectLockDeleteMultipartObjectWithRetention, markerDBStore),
		b.add("object_lock_delete_object_with_retention_and_marker",
			objectLockDeleteObjectWithRetentionAndMarker, markerDBStore),
		b.add("object_lock_multi_delete_object_with_retention", objectLockMultiDeleteObjectWithRetention, markerDBStore),
		b.add("object_lock_put_legal_hold", objectLockPutLegalHold, markerDBStore),
		b.add("object_lock_put_legal_hold_invalid_bucket", objectLockPutLegalHoldInvalidBucket),
		b.add("object_lock_put_legal_hold_invalid_status", objectLockPutLegalHoldInvalidStatus, markerDBStore),
		b.add("object_lock_get_legal_hold", objectLockGetLegalHold, markerDBStore),
		b.add("object_lock_get_legal_hold_invalid_bucket", objectLockGetLegalHoldInvalidBucket),
		b.add("object_lock_delete_object_with_legal_hold_on", objectLockDeleteObjectWithLegalHoldOn, markerDBStore),
		b.add("object_lock_delete_multipart_object_with_legal_hold_on",
			objectLockDeleteMultipartObjectWithLegalHoldOn, markerDBStore),
		b.add("object_lock_delete_object_with_legal_hold_off", objectLockDeleteObjectWithLegalHoldOff, markerDBStore),
		b.add("object_lock_get_obj_metadata", objectLockGetObjMetadata, markerDBStore),
		b.add("object_lock_uploading_obj", objectLockUploadingObj, markerDBStore),
		b.add("object_lock_changing_mode_from_governance_with_bypass",
			objectLockChangingModeFromGovernanceWithBypass, markerDBStore),
		b.add("object_lock_changing_mode_from_governance_without_bypass",
			objectLockChangingModeFromGovernanceWithoutBypass, markerDBStore),
		b.add("object_lock_changing_mode_from_compliance", objectLockChangingModeFromCompliance, markerDBStore),
	}
}

// lockKey is the object every test in this file writes, as upstream.
const lockKey = "file1"

// newObjectLockBucket creates a bucket with object lock enabled, which also
// enables versioning.
func newObjectLockBucket(e *fixture.Env) string {
	name := e.NewBucketName()
	_, err := e.Client().CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket:                     aws.String(name),
		ObjectLockEnabledForBucket: aws.Bool(true),
	})
	s3util.NoError(e.T, err, "create bucket with object lock")
	e.T.Cleanup(func() { e.Nuke(e.Client(), name) })
	return name
}

// newLockedBucket is newObjectLockBucket with one object already written.
func newLockedBucket(e *fixture.Env) string {
	bucket := newObjectLockBucket(e)
	putObject(e, bucket, lockKey, "abc")
	return bucket
}

// lockConf builds the default-retention configuration upstream writes inline
// in most of these tests. Days and years are pointers because the two are
// alternatives, and one test sends both to see it refused.
func lockConf(mode types.ObjectLockRetentionMode, days, years *int32) *types.ObjectLockConfiguration {
	return &types.ObjectLockConfiguration{
		ObjectLockEnabled: types.ObjectLockEnabledEnabled,
		Rule: &types.ObjectLockRule{
			DefaultRetention: &types.DefaultRetention{Mode: mode, Days: days, Years: years},
		},
	}
}

// putLockConf sets the bucket's default retention and returns the error, since
// half of these tests are about the error.
func putLockConf(e *fixture.Env, bucket string, conf *types.ObjectLockConfiguration) error {
	_, err := e.Client().PutObjectLockConfiguration(e.Ctx(), &awss3.PutObjectLockConfigurationInput{
		Bucket:                  aws.String(bucket),
		ObjectLockConfiguration: conf,
	})
	return err
}

// retention builds an object retention, mirroring upstream's
//
//	{'Mode': 'GOVERNANCE', 'RetainUntilDate': datetime.datetime(...)}
func retention(mode types.ObjectLockRetentionMode, until time.Time) *types.ObjectLockRetention {
	return &types.ObjectLockRetention{Mode: mode, RetainUntilDate: aws.Time(until)}
}

// The retention dates upstream hard-codes, named rather than repeated: a
// time.Time cannot be a constant.
func jan2030() time.Time  { return time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC) }
func jan32030() time.Time { return time.Date(2030, time.January, 3, 0, 0, 0, 0, time.UTC) }
func jan2140() time.Time  { return time.Date(2140, time.January, 1, 0, 0, 0, 0, time.UTC) }

// putRetention sets an object's retention and returns the error.
func putRetention(e *fixture.Env, bucket string, r *types.ObjectLockRetention,
	opts ...func(*awss3.PutObjectRetentionInput),
) error {
	in := &awss3.PutObjectRetentionInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		Retention: r,
	}
	for _, opt := range opts {
		opt(in)
	}
	_, err := e.Client().PutObjectRetention(e.Ctx(), in)
	return err
}

// getRetention reads back an object's retention.
func getRetention(e *fixture.Env, bucket, versionID string) *types.ObjectLockRetention {
	in := &awss3.GetObjectRetentionInput{Bucket: aws.String(bucket), Key: aws.String(lockKey)}
	if versionID != "" {
		in.VersionId = aws.String(versionID)
	}
	out, err := e.Client().GetObjectRetention(e.Ctx(), in)
	s3util.NoError(e.T, err, "get object retention")
	return out.Retention
}

// equalRetention compares a retention against what was asked for. Times are
// compared as RFC3339 text: time.Time carries a monotonic reading and a
// location that never survive a round trip, so == on the value is wrong.
func equalRetention(e *fixture.Env, got, want *types.ObjectLockRetention) {
	if got == nil {
		e.T.Fatalf("retention is absent, want mode %s", want.Mode)
	}
	s3util.Equal(e.T, got.Mode, want.Mode, "retention mode")
	s3util.Equal(e.T,
		aws.ToTime(got.RetainUntilDate).UTC().Format(time.RFC3339),
		aws.ToTime(want.RetainUntilDate).UTC().Format(time.RFC3339),
		"retain until date")
}

// putLegalHold sets an object's legal hold and returns the error.
func putLegalHold(e *fixture.Env, bucket string, status types.ObjectLockLegalHoldStatus) error {
	_, err := e.Client().PutObjectLegalHold(e.Ctx(), &awss3.PutObjectLegalHoldInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		LegalHold: &types.ObjectLockLegalHold{Status: status},
	})
	return err
}

// deleteVersionBypass deletes one version, bypassing governance retention, and
// returns the error. Tests end with this so the object does not have to be
// waited out during cleanup.
func deleteVersionBypass(e *fixture.Env, bucket, versionID string) error {
	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		VersionId:                 aws.String(versionID),
		BypassGovernanceRetention: aws.Bool(true),
	})
	return err
}

func objectLockPutObjLock(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	s3util.NoError(e.T, putLockConf(e, bucket,
		lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), nil)),
		"put object lock configuration")
	s3util.NoError(e.T, putLockConf(e, bucket,
		lockConf(types.ObjectLockRetentionModeCompliance, nil, aws.Int32(1))),
		"put object lock configuration")

	// Enabling object lock enables versioning with it.
	out, err := e.Client().GetBucketVersioning(e.Ctx(), &awss3.GetBucketVersioningInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get bucket versioning")
	s3util.Equal(e.T, out.Status, types.BucketVersioningStatusEnabled, "versioning status")
}

func objectLockPutObjLockInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()

	err := putLockConf(e, bucket, lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), nil))
	s3util.ErrorIs(e.T, err, 409, "InvalidBucketState")
}

func objectLockPutObjLockEnableAfterCreate(e *fixture.Env) {
	bucket := e.NewBucket()
	conf := lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), nil)

	// Unversioned.
	s3util.ErrorIs(e.T, putLockConf(e, bucket, conf), 409, "InvalidBucketState")

	// Versioning suspended.
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusSuspended)
	s3util.ErrorIs(e.T, putLockConf(e, bucket, conf), 409, "InvalidBucketState")

	// Versioning enabled.
	configureVersioningRetry(e, bucket, types.BucketVersioningStatusEnabled)
	s3util.NoError(e.T, putLockConf(e, bucket, conf), "put object lock configuration")
}

func objectLockPutObjLockWithDaysAndYears(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	err := putLockConf(e, bucket, lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), aws.Int32(1)))
	s3util.ErrorIs(e.T, err, 400, "MalformedXML")
}

func objectLockPutObjLockInvalidDays(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	err := putLockConf(e, bucket, lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(0), nil))
	s3util.ErrorIs(e.T, err, 400, "InvalidRetentionPeriod")
}

func objectLockPutObjLockInvalidYears(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	err := putLockConf(e, bucket, lockConf(types.ObjectLockRetentionModeGovernance, nil, aws.Int32(-1)))
	s3util.ErrorIs(e.T, err, 400, "InvalidRetentionPeriod")
}

func objectLockPutObjLockInvalidMode(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	// Neither an unknown mode nor the right one in the wrong case is valid;
	// the SDK's mode is a plain string type, so both go on the wire as given.
	for _, mode := range []types.ObjectLockRetentionMode{"abc", "governance"} {
		err := putLockConf(e, bucket, lockConf(mode, nil, aws.Int32(1)))
		s3util.ErrorIs(e.T, err, 400, "MalformedXML")
	}
}

func objectLockPutObjLockInvalidStatus(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	conf := lockConf(types.ObjectLockRetentionModeGovernance, nil, aws.Int32(1))
	conf.ObjectLockEnabled = "Disabled"
	s3util.ErrorIs(e.T, putLockConf(e, bucket, conf), 400, "MalformedXML")
}

func objectLockSuspendVersioning(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	_, err := e.Client().PutBucketVersioning(e.Ctx(), &awss3.PutBucketVersioningInput{
		Bucket:                  aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusSuspended},
	})
	s3util.ErrorIs(e.T, err, 409, "InvalidBucketState")
}

func objectLockGetObjLock(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	conf := lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), nil)
	s3util.NoError(e.T, putLockConf(e, bucket, conf), "put object lock configuration")

	out, err := e.Client().GetObjectLockConfiguration(e.Ctx(), &awss3.GetObjectLockConfigurationInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get object lock configuration")

	got := out.ObjectLockConfiguration
	s3util.EqualNow(e.T, got != nil && got.Rule != nil && got.Rule.DefaultRetention != nil, true,
		"configuration carries a default retention")
	s3util.Equal(e.T, got.ObjectLockEnabled, conf.ObjectLockEnabled, "object lock enabled")
	s3util.Equal(e.T, got.Rule.DefaultRetention.Mode, conf.Rule.DefaultRetention.Mode, "default retention mode")
	s3util.Equal(e.T, aws.ToInt32(got.Rule.DefaultRetention.Days), int32(1), "default retention days")
}

func objectLockGetObjLockInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().GetObjectLockConfiguration(e.Ctx(), &awss3.GetObjectLockConfigurationInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 404, "ObjectLockConfigurationNotFoundError")
}

func objectLockPutObjRetention(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	want := retention(types.ObjectLockRetentionModeGovernance, jan2140())
	s3util.NoError(e.T, putRetention(e, bucket, want), "put object retention")
	equalRetention(e, getRetention(e, bucket, ""), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockPutObjRetentionInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, lockKey, "abc")

	err := putRetention(e, bucket, retention(types.ObjectLockRetentionModeGovernance, jan2030()))
	s3util.ErrorIs(e.T, err, 400, "InvalidRequest")
}

func objectLockPutObjRetentionInvalidMode(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	putObject(e, bucket, lockKey, "abc")

	for _, mode := range []types.ObjectLockRetentionMode{"governance", "abc"} {
		err := putRetention(e, bucket, retention(mode, jan2030()))
		s3util.ErrorIs(e.T, err, 400, "MalformedXML")
	}
}

func objectLockGetObjRetention(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	want := retention(types.ObjectLockRetentionModeGovernance, jan2030())
	s3util.NoError(e.T, putRetention(e, bucket, want), "put object retention")
	equalRetention(e, getRetention(e, bucket, ""), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockGetObjRetentionISO8601(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")
	versionID := mustField(e, put.VersionId, "VersionId")

	until := time.Now().UTC().AddDate(0, 0, 365)
	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, until)), "put object retention")

	// Upstream reads x-amz-object-lock-retain-until-date off the raw response
	// and parses it as ISO 8601. The SDK's deserializer parses that header the
	// same way, so a HEAD that succeeds and carries a date is the same
	// assertion: a value in any other format would have failed to deserialize.
	out, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, out.ObjectLockRetainUntilDate != nil, true, "retain until date is set")

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, versionID), "delete object")
}

func objectLockGetObjRetentionInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, lockKey, "abc")

	_, err := e.Client().GetObjectRetention(e.Ctx(), &awss3.GetObjectRetentionInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(lockKey),
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidRequest")
}

func objectLockPutObjRetentionVersionID(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	putObject(e, bucket, lockKey, "abc")
	put := putObject(e, bucket, lockKey, "abc")
	versionID := mustField(e, put.VersionId, "VersionId")

	want := retention(types.ObjectLockRetentionModeGovernance, jan2030())
	s3util.NoError(e.T, putRetention(e, bucket, want, func(in *awss3.PutObjectRetentionInput) {
		in.VersionId = aws.String(versionID)
	}), "put object retention")
	equalRetention(e, getRetention(e, bucket, versionID), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, versionID), "delete object")
}

func objectLockPutObjRetentionOverrideDefaultRetention(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	s3util.NoError(e.T, putLockConf(e, bucket,
		lockConf(types.ObjectLockRetentionModeGovernance, aws.Int32(1), nil)),
		"put object lock configuration")

	put := putObject(e, bucket, lockKey, "abc")
	want := retention(types.ObjectLockRetentionModeGovernance, jan2030())
	s3util.NoError(e.T, putRetention(e, bucket, want), "put object retention")
	equalRetention(e, getRetention(e, bucket, ""), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockPutObjRetentionIncreasePeriod(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan2030())),
		"put object retention")

	// Lengthening a governance retention needs no bypass.
	want := retention(types.ObjectLockRetentionModeGovernance, jan32030())
	s3util.NoError(e.T, putRetention(e, bucket, want), "put longer object retention")
	equalRetention(e, getRetention(e, bucket, ""), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockPutObjRetentionShortenPeriod(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan32030())),
		"put object retention")

	err := putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan2030()))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockPutObjRetentionShortenPeriodBypass(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan32030())),
		"put object retention")

	want := retention(types.ObjectLockRetentionModeGovernance, jan2030())
	s3util.NoError(e.T, putRetention(e, bucket, want, func(in *awss3.PutObjectRetentionInput) {
		in.BypassGovernanceRetention = aws.Bool(true)
	}), "put shorter object retention with bypass")
	equalRetention(e, getRetention(e, bucket, ""), want)

	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, put.VersionId, "VersionId")), "delete object")
}

func objectLockDeleteObjectWithRetention(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")
	versionID := mustField(e, put.VersionId, "VersionId")

	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan2030())),
		"put object retention")

	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		VersionId:                 aws.String(versionID),
		BypassGovernanceRetention: aws.Bool(true),
	})
	s3util.NoError(e.T, err, "delete object with bypass")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")
}

func objectLockDeleteMultipartObjectWithRetention(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	versionID := lockedMultipartUpload(e, bucket, lockKey, func(in *awss3.CreateMultipartUploadInput) {
		in.ObjectLockMode = types.ObjectLockModeGovernance
		in.ObjectLockRetainUntilDate = aws.Time(jan2030())
	})

	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		VersionId:                 aws.String(versionID),
		BypassGovernanceRetention: aws.Bool(true),
	})
	s3util.NoError(e.T, err, "delete object with bypass")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")
}

// lockedMultipartUpload writes a one-part object whose create call carries
// object lock settings, and returns the completed object's version.
func lockedMultipartUpload(e *fixture.Env, bucket, lockKey string,
	opt func(*awss3.CreateMultipartUploadInput),
) string {
	in := &awss3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(lockKey)}
	opt(in)
	created, err := e.Client().CreateMultipartUpload(e.Ctx(), in)
	s3util.NoError(e.T, err, "create multipart upload")

	part, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(lockKey),
		UploadId:   created.UploadId,
		PartNumber: aws.Int32(1),
		Body:       readerOf("abc"),
	})
	s3util.NoError(e.T, err, "upload part")

	done, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(lockKey),
		UploadId: created.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{{ETag: part.ETag, PartNumber: aws.Int32(1)}},
		},
	})
	s3util.NoError(e.T, err, "complete multipart upload")
	return mustField(e, done.VersionId, "VersionId")
}

func objectLockDeleteObjectWithRetentionAndMarker(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")
	versionID := mustField(e, put.VersionId, "VersionId")

	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan2030())),
		"put object retention")

	// A plain delete leaves a marker and does not touch the locked version.
	marker, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(lockKey),
	})
	s3util.NoError(e.T, err, "delete object")

	_, err = e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	// Removing the marker does not unlock it either.
	_, err = e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(mustField(e, marker.VersionId, "VersionId")),
	})
	s3util.NoError(e.T, err, "delete marker")

	_, err = e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		VersionId:                 aws.String(versionID),
		BypassGovernanceRetention: aws.Bool(true),
	})
	s3util.NoError(e.T, err, "delete object with bypass")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")
}

func objectLockMultiDeleteObjectWithRetention(e *fixture.Env) {
	const key2 = "file2"
	bucket := newObjectLockBucket(e)
	version1 := mustField(e, putObject(e, bucket, lockKey, "abc").VersionId, "VersionId")
	version2 := mustField(e, putObject(e, bucket, key2, "abc").VersionId, "VersionId")

	// file1 is under retention, file2 is not.
	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeGovernance, jan2030())),
		"put object retention")

	out, err := e.Client().DeleteObjects(e.Ctx(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: []types.ObjectIdentifier{
			{Key: aws.String(lockKey), VersionId: aws.String(version1)},
			{Key: aws.String(key2), VersionId: aws.String(version2)},
		}},
	})
	s3util.NoError(e.T, err, "delete objects")
	s3util.EqualNow(e.T, len(out.Deleted), 1, "deleted count")
	s3util.EqualNow(e.T, len(out.Errors), 1, "error count")

	s3util.Equal(e.T, aws.ToString(out.Errors[0].Code), "AccessDenied", "error code")
	s3util.Equal(e.T, aws.ToString(out.Errors[0].Key), lockKey, "error key")
	s3util.Equal(e.T, aws.ToString(out.Errors[0].VersionId), version1, "error version")
	s3util.Equal(e.T, aws.ToString(out.Deleted[0].Key), key2, "deleted key")
	s3util.Equal(e.T, aws.ToString(out.Deleted[0].VersionId), version2, "deleted version")

	out, err = e.Client().DeleteObjects(e.Ctx(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: []types.ObjectIdentifier{
			{Key: aws.String(lockKey), VersionId: aws.String(version1)},
		}},
		BypassGovernanceRetention: aws.Bool(true),
	})
	s3util.NoError(e.T, err, "delete objects with bypass")
	s3util.Equal(e.T, len(out.Errors), 0, "error count")
	s3util.EqualNow(e.T, len(out.Deleted), 1, "deleted count")
	s3util.Equal(e.T, aws.ToString(out.Deleted[0].Key), lockKey, "deleted key")
	s3util.Equal(e.T, aws.ToString(out.Deleted[0].VersionId), version1, "deleted version")
}

func objectLockPutLegalHold(e *fixture.Env) {
	bucket := newLockedBucket(e)

	for _, status := range []types.ObjectLockLegalHoldStatus{
		types.ObjectLockLegalHoldStatusOn,
		types.ObjectLockLegalHoldStatusOff,
	} {
		out, err := e.Client().PutObjectLegalHold(e.Ctx(), &awss3.PutObjectLegalHoldInput{
			Bucket:    aws.String(bucket),
			Key:       aws.String(lockKey),
			LegalHold: &types.ObjectLockLegalHold{Status: status},
		})
		s3util.NoError(e.T, err, "put object legal hold")
		s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
	}
}

func objectLockPutLegalHoldInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, lockKey, "abc")

	err := putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOn)
	s3util.ErrorIs(e.T, err, 400, "InvalidRequest")
}

func objectLockPutLegalHoldInvalidStatus(e *fixture.Env) {
	bucket := newLockedBucket(e)

	s3util.ErrorIs(e.T, putLegalHold(e, bucket, "abc"), 400, "MalformedXML")
}

func objectLockGetLegalHold(e *fixture.Env) {
	bucket := newLockedBucket(e)

	for _, status := range []types.ObjectLockLegalHoldStatus{
		types.ObjectLockLegalHoldStatusOn,
		types.ObjectLockLegalHoldStatusOff,
	} {
		s3util.NoError(e.T, putLegalHold(e, bucket, status), "put object legal hold")

		out, err := e.Client().GetObjectLegalHold(e.Ctx(), &awss3.GetObjectLegalHoldInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(lockKey),
		})
		s3util.NoError(e.T, err, "get object legal hold")
		s3util.Equal(e.T, out.LegalHold.Status, status, "legal hold status")
	}
}

func objectLockGetLegalHoldInvalidBucket(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, lockKey, "abc")

	_, err := e.Client().GetObjectLegalHold(e.Ctx(), &awss3.GetObjectLegalHoldInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(lockKey),
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidRequest")
}

func objectLockDeleteObjectWithLegalHoldOn(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOn), "put object legal hold")

	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(mustField(e, put.VersionId, "VersionId")),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	// A legal hold cannot be bypassed, so it has to come off here or the
	// bucket would outlive the test.
	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOff), "clear object legal hold")
}

func objectLockDeleteMultipartObjectWithLegalHoldOn(e *fixture.Env) {
	bucket := newObjectLockBucket(e)

	versionID := lockedMultipartUpload(e, bucket, lockKey, func(in *awss3.CreateMultipartUploadInput) {
		in.ObjectLockLegalHoldStatus = types.ObjectLockLegalHoldStatusOn
	})

	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(versionID),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOff), "clear object legal hold")
}

func objectLockDeleteObjectWithLegalHoldOff(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	put := putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOff), "put object legal hold")

	out, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(lockKey),
		VersionId: aws.String(mustField(e, put.VersionId, "VersionId")),
	})
	s3util.NoError(e.T, err, "delete object")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")
}

func objectLockGetObjMetadata(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	putObject(e, bucket, lockKey, "abc")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOn), "put object legal hold")
	want := retention(types.ObjectLockRetentionModeGovernance, jan2030())
	s3util.NoError(e.T, putRetention(e, bucket, want), "put object retention")

	out, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(lockKey),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, string(out.ObjectLockMode), string(want.Mode), "object lock mode")
	s3util.Equal(e.T,
		aws.ToTime(out.ObjectLockRetainUntilDate).UTC().Format(time.RFC3339),
		aws.ToTime(want.RetainUntilDate).UTC().Format(time.RFC3339),
		"retain until date")
	s3util.Equal(e.T, out.ObjectLockLegalHoldStatus, types.ObjectLockLegalHoldStatusOn, "legal hold status")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOff), "clear object legal hold")
	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, out.VersionId, "VersionId")), "delete object")
}

func objectLockUploadingObj(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	until := jan2030()

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		Body:                      readerOf("abc"),
		ObjectLockMode:            types.ObjectLockModeGovernance,
		ObjectLockRetainUntilDate: aws.Time(until),
		ObjectLockLegalHoldStatus: types.ObjectLockLegalHoldStatusOn,
	})
	s3util.NoError(e.T, err, "put object with lock settings")

	out, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(lockKey),
	})
	s3util.NoError(e.T, err, "head object")
	s3util.Equal(e.T, out.ObjectLockMode, types.ObjectLockModeGovernance, "object lock mode")
	s3util.Equal(e.T,
		aws.ToTime(out.ObjectLockRetainUntilDate).UTC().Format(time.RFC3339),
		until.Format(time.RFC3339),
		"retain until date")
	s3util.Equal(e.T, out.ObjectLockLegalHoldStatus, types.ObjectLockLegalHoldStatusOn, "legal hold status")

	s3util.NoError(e.T, putLegalHold(e, bucket, types.ObjectLockLegalHoldStatusOff), "clear object legal hold")
	s3util.NoError(e.T, deleteVersionBypass(e, bucket, mustField(e, out.VersionId, "VersionId")), "delete object")
}

// putUnderGovernance writes an object locked in the given mode until ten
// seconds from now, the shape the three mode-changing tests share. The window
// is short so cleanup can wait out a compliance lock rather than give up.
func putUnderGovernance(e *fixture.Env, bucket string, mode types.ObjectLockMode) time.Time {
	until := time.Now().UTC().Add(10 * time.Second)
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(lockKey),
		Body:                      readerOf("abc"),
		ObjectLockMode:            mode,
		ObjectLockRetainUntilDate: aws.Time(until),
	})
	s3util.NoError(e.T, err, "put object under retention")
	return until
}

func objectLockChangingModeFromGovernanceWithBypass(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	until := putUnderGovernance(e, bucket, types.ObjectLockModeGovernance)

	// Governance can be tightened to compliance with the bypass header.
	s3util.NoError(e.T, putRetention(e, bucket,
		retention(types.ObjectLockRetentionModeCompliance, until),
		func(in *awss3.PutObjectRetentionInput) { in.BypassGovernanceRetention = aws.Bool(true) }),
		"put object retention")
}

func objectLockChangingModeFromGovernanceWithoutBypass(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	until := putUnderGovernance(e, bucket, types.ObjectLockModeGovernance)

	err := putRetention(e, bucket, retention(types.ObjectLockRetentionModeCompliance, until))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectLockChangingModeFromCompliance(e *fixture.Env) {
	bucket := newObjectLockBucket(e)
	until := putUnderGovernance(e, bucket, types.ObjectLockModeCompliance)

	// Compliance cannot be loosened at all, bypass or not.
	err := putRetention(e, bucket, retention(types.ObjectLockRetentionModeGovernance, until))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}
