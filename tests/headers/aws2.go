package headers

import (
	"strconv"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerAuthAWS2 is on every test in this file, matching upstream. It selects
// the header tests that only mean anything under the v2 signature, which signs
// the date and lets a bad one reach the server intact.
const markerAuthAWS2 = "auth_aws2"

// Dates upstream sends as x-amz-date. The v2 signer covers the header, so the
// server sees a valid signature over a date it has to judge on its own.
const (
	dateBeforeToday = "Tue, 07 Jul 2010 21:53:04 GMT"
	dateAfterToday  = "Tue, 07 Jul 2030 21:53:04 GMT"
	dateBeforeEpoch = "Tue, 07 Jul 1950 21:53:04 GMT"
	dateAfterEnd    = "Tue, 07 Jul 9999 21:53:04 GMT"
)

func aws2Tests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_create_bad_md5_invalid_garbage_aws2", objectCreateBadMD5InvalidGarbageAWS2, markerAuthAWS2),
		b.add("object_create_bad_contentlength_mismatch_below_aws2", objectCreateBadContentlengthMismatchBelowAWS2,
			markerAuthAWS2, "fails_on_rgw"),
		b.add("object_create_bad_authorization_incorrect_aws2", objectCreateBadAuthorizationIncorrectAWS2,
			markerAuthAWS2, "fails_on_rgw"),
		b.add("object_create_bad_authorization_invalid_aws2", objectCreateBadAuthorizationInvalidAWS2,
			markerAuthAWS2, "fails_on_rgw"),
		b.add("object_create_bad_ua_empty_aws2", objectCreateBadUAEmptyAWS2, markerAuthAWS2),
		b.add("object_create_bad_ua_none_aws2", objectCreateBadUANoneAWS2, markerAuthAWS2),
		b.add("object_create_bad_date_invalid_aws2", objectCreateBadDateInvalidAWS2, markerAuthAWS2),
		b.add("object_create_bad_date_empty_aws2", objectCreateBadDateEmptyAWS2, markerAuthAWS2),
		b.add("object_create_bad_date_none_aws2", objectCreateBadDateNoneAWS2, markerAuthAWS2, "fails_on_rgw"),
		b.add("object_create_bad_date_before_today_aws2", objectCreateBadDateBeforeTodayAWS2, markerAuthAWS2),
		b.add("object_create_bad_date_before_epoch_aws2", objectCreateBadDateBeforeEpochAWS2, markerAuthAWS2),
		b.add("object_create_bad_date_after_end_aws2", objectCreateBadDateAfterEndAWS2, markerAuthAWS2),
		b.add("bucket_create_bad_authorization_invalid_aws2", bucketCreateBadAuthorizationInvalidAWS2,
			markerAuthAWS2, "fails_on_rgw"),
		b.add("bucket_create_bad_ua_empty_aws2", bucketCreateBadUAEmptyAWS2, markerAuthAWS2, "fails_on_rgw"),
		b.add("bucket_create_bad_ua_none_aws2", bucketCreateBadUANoneAWS2, markerAuthAWS2, "fails_on_rgw"),
		b.add("bucket_create_bad_date_invalid_aws2", bucketCreateBadDateInvalidAWS2, markerAuthAWS2),
		b.add("bucket_create_bad_date_empty_aws2", bucketCreateBadDateEmptyAWS2, markerAuthAWS2),
		b.add("bucket_create_bad_date_none_aws2", bucketCreateBadDateNoneAWS2, markerAuthAWS2, "fails_on_rgw"),
		b.add("bucket_create_bad_date_before_today_aws2", bucketCreateBadDateBeforeTodayAWS2, markerAuthAWS2),
		b.add("bucket_create_bad_date_after_today_aws2", bucketCreateBadDateAfterTodayAWS2, markerAuthAWS2),
		b.add("bucket_create_bad_date_before_epoch_aws2", bucketCreateBadDateBeforeEpochAWS2, markerAuthAWS2),
	}
}

func objectCreateBadMD5InvalidGarbageAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"Content-MD5": "AWS HAHAHA"}))
	s3util.ErrorIs(e.T, err, 400, "InvalidDigest")
}

func objectCreateBadContentlengthMismatchBelowAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), e.WithContentLength(strconv.Itoa(len(content)-1)))
	s3util.ErrorIs(e.T, err, 400, "BadDigest")
}

func objectCreateBadAuthorizationIncorrectAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{
		"Authorization": "AWS AKIAIGR7ZNNBHC5BKSUB:FWeDfwojDSdS2Ztmpfeubhd9isU=",
	}))
	s3util.ErrorIs(e.T, err, 403, "InvalidDigest")
}

func objectCreateBadAuthorizationInvalidAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"Authorization": "AWS HAHAHA"}))
	s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
}

func objectCreateBadUAEmptyAWS2(e *fixture.Env) {
	bucket := createObject(e, e.V2Client(), client.WithHeaders(map[string]string{"User-Agent": ""}))
	putObject(e, e.V2Client(), bucket)
}

func objectCreateBadUANoneAWS2(e *fixture.Env) {
	bucket := createObject(e, e.V2Client(), client.WithoutHeader("User-Agent"))
	putObject(e, e.V2Client(), bucket)
}

func objectCreateBadDateInvalidAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": "Bad Date"}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectCreateBadDateEmptyAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": ""}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectCreateBadDateNoneAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithoutHeader("x-amz-date"))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectCreateBadDateBeforeTodayAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateBeforeToday}))
	s3util.ErrorIs(e.T, err, 403, "RequestTimeTooSkewed")
}

func objectCreateBadDateBeforeEpochAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateBeforeEpoch}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectCreateBadDateAfterEndAWS2(e *fixture.Env) {
	err := createBadObject(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateAfterEnd}))
	s3util.ErrorIs(e.T, err, 403, "RequestTimeTooSkewed")
}

func bucketCreateBadAuthorizationInvalidAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"Authorization": "AWS HAHAHA"}))
	s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
}

func bucketCreateBadUAEmptyAWS2(e *fixture.Env) {
	createBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"User-Agent": ""}))
}

func bucketCreateBadUANoneAWS2(e *fixture.Env) {
	createBucket(e, e.V2Client(), client.WithoutHeader("User-Agent"))
}

func bucketCreateBadDateInvalidAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": "Bad Date"}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketCreateBadDateEmptyAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": ""}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketCreateBadDateNoneAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithoutHeader("x-amz-date"))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketCreateBadDateBeforeTodayAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateBeforeToday}))
	s3util.ErrorIs(e.T, err, 403, "RequestTimeTooSkewed")
}

func bucketCreateBadDateAfterTodayAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateAfterToday}))
	s3util.ErrorIs(e.T, err, 403, "RequestTimeTooSkewed")
}

func bucketCreateBadDateBeforeEpochAWS2(e *fixture.Env) {
	err := createBadBucket(e, e.V2Client(), client.WithHeaders(map[string]string{"x-amz-date": dateBeforeEpoch}))
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}
