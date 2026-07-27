package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func listingTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_list_delimiter_alt", bucketListDelimiterAlt),
		b.add("bucket_list_delimiter_dot", bucketListDelimiterDot),
		b.add("bucket_list_delimiter_empty", bucketListDelimiterEmpty),
		b.add("bucket_list_delimiter_none", bucketListDelimiterNone),
		b.add("bucket_list_delimiter_not_exist", bucketListDelimiterNotExist),
		b.add("bucket_list_delimiter_percentage", bucketListDelimiterPercentage),
		b.add("bucket_list_delimiter_prefix", bucketListDelimiterPrefix, "fails_on_dbstore"),
		b.add("bucket_list_delimiter_prefix_underscore", bucketListDelimiterPrefixUnderscore, "fails_on_dbstore"),
		b.add("bucket_list_delimiter_unreadable", bucketListDelimiterUnreadable),
		b.add("bucket_list_delimiter_whitespace", bucketListDelimiterWhitespace),
		b.add("bucket_list_encoding_basic", bucketListEncodingBasic),
		b.add("bucket_list_long_name", bucketListLongName),
		b.add("bucket_list_many", bucketListMany, "fails_on_dbstore"),
		b.add("bucket_list_marker_after_list", bucketListMarkerAfterList),
		b.add("bucket_list_marker_not_in_list", bucketListMarkerNotInList),
		b.add("bucket_list_marker_unreadable", bucketListMarkerUnreadable),
		b.add("bucket_list_maxkeys_invalid", bucketListMaxkeysInvalid),
		b.add("bucket_list_maxkeys_none", bucketListMaxkeysNone),
		b.add("bucket_list_maxkeys_one", bucketListMaxkeysOne, "fails_on_dbstore"),
		b.add("bucket_list_maxkeys_zero", bucketListMaxkeysZero),
		b.add("bucket_list_objects_anonymous_fail", bucketListObjectsAnonymousFail),
		b.add("bucket_list_prefix_alt", bucketListPrefixAlt),
		b.add("bucket_list_prefix_basic", bucketListPrefixBasic),
		b.add("bucket_list_prefix_delimiter_alt", bucketListPrefixDelimiterAlt),
		b.add("bucket_list_prefix_delimiter_basic", bucketListPrefixDelimiterBasic),
		b.add("bucket_list_prefix_delimiter_delimiter_not_exist", bucketListPrefixDelimiterDelimiterNotExist),
		b.add("bucket_list_prefix_delimiter_prefix_delimiter_not_exist", bucketListPrefixDelimiterPrefixDelimiterNotExist),
		b.add("bucket_list_prefix_delimiter_prefix_not_exist", bucketListPrefixDelimiterPrefixNotExist),
		b.add("bucket_list_prefix_empty", bucketListPrefixEmpty),
		b.add("bucket_list_prefix_none", bucketListPrefixNone),
		b.add("bucket_list_prefix_not_exist", bucketListPrefixNotExist),
		b.add("bucket_list_prefix_unreadable", bucketListPrefixUnreadable),
		b.add("bucket_list_return_data", bucketListReturnData, "fails_on_dbstore"),
		b.add("bucket_list_special_prefix", bucketListSpecialPrefix),
	}
}

// listKeys returns the object keys from a ListObjects response, mirroring
// upstream's _get_keys.
func listKeys(out *awss3.ListObjectsOutput) []string {
	keys := make([]string, 0, len(out.Contents))
	for _, o := range out.Contents {
		keys = append(keys, aws.ToString(o.Key))
	}
	return keys
}

// listPrefixes returns the common prefixes, mirroring upstream's _get_prefixes.
func listPrefixes(out *awss3.ListObjectsOutput) []string {
	prefixes := make([]string, 0, len(out.CommonPrefixes))
	for _, p := range out.CommonPrefixes {
		prefixes = append(prefixes, aws.ToString(p.Prefix))
	}
	return prefixes
}

// listObjects runs ListObjects and fails the test if it errors.
func listObjects(e *fixture.Env, in *awss3.ListObjectsInput) *awss3.ListObjectsOutput {
	out, err := e.Client().ListObjects(e.Ctx(), in)
	s3util.NoError(e.T, err, "list objects")
	return out
}

func bucketListDelimiterAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "cab", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "a", "delimiter")
	// foo contains no 'a' and so is a complete key.
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo"}, "keys")
	// bar, baz and cab are broken up by the 'a' delimiters.
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"ba", "ca"}, "prefixes")
}

func bucketListDelimiterDot(e *fixture.Env) {
	bucket := createObjects(e, "b.ar", "b.az", "c.ab", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("."),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), ".", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"b.", "c."}, "prefixes")
}

func bucketListDelimiterEmpty(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(""),
	})
	// An empty delimiter is not echoed back.
	s3util.Equal(e.T, out.Delimiter == nil, true, "delimiter absent")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListDelimiterNone(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, out.Delimiter == nil, true, "delimiter absent")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListDelimiterNotExist(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListDelimiterPercentage(e *fixture.Env) {
	bucket := createObjects(e, "b%ar", "b%az", "c%ab", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("%"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "%", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"b%", "c%"}, "prefixes")
}

func bucketListDelimiterPrefix(e *fixture.Env) {
	bucket := createObjects(e, "asdf", "boo/bar", "boo/baz/xyzzy", "cquux/thud", "cquux/bla")

	const delim = "/"
	prefix := ""

	marker := validateBucketList(e, bucket, prefix, delim, "", 1, true, []string{"asdf"}, nil, "asdf")
	marker = validateBucketList(e, bucket, prefix, delim, marker, 1, true, nil, []string{"boo/"}, "boo/")
	validateBucketList(e, bucket, prefix, delim, marker, 1, false, nil, []string{"cquux/"}, "")

	marker = validateBucketList(e, bucket, prefix, delim, "", 2, true, []string{"asdf"}, []string{"boo/"}, "boo/")
	validateBucketList(e, bucket, prefix, delim, marker, 2, false, nil, []string{"cquux/"}, "")

	prefix = "boo/"

	marker = validateBucketList(e, bucket, prefix, delim, "", 1, true, []string{"boo/bar"}, nil, "boo/bar")
	validateBucketList(e, bucket, prefix, delim, marker, 1, false, nil, []string{"boo/baz/"}, "")

	validateBucketList(e, bucket, prefix, delim, "", 2, false, []string{"boo/bar"}, []string{"boo/baz/"}, "")
}

func bucketListDelimiterPrefixUnderscore(e *fixture.Env) {
	bucket := createObjects(e, "_obj1_", "_under1/bar", "_under1/baz/xyzzy", "_under2/thud", "_under2/bla")

	const delim = "/"
	prefix := ""

	marker := validateBucketList(e, bucket, prefix, delim, "", 1, true, []string{"_obj1_"}, nil, "_obj1_")
	marker = validateBucketList(e, bucket, prefix, delim, marker, 1, true, nil, []string{"_under1/"}, "_under1/")
	validateBucketList(e, bucket, prefix, delim, marker, 1, false, nil, []string{"_under2/"}, "")

	marker = validateBucketList(e, bucket, prefix, delim, "", 2, true, []string{"_obj1_"}, []string{"_under1/"}, "_under1/")
	validateBucketList(e, bucket, prefix, delim, marker, 2, false, nil, []string{"_under2/"}, "")

	prefix = "_under1/"

	marker = validateBucketList(e, bucket, prefix, delim, "", 1, true, []string{"_under1/bar"}, nil, "_under1/bar")
	validateBucketList(e, bucket, prefix, delim, marker, 1, false, nil, []string{"_under1/baz/"}, "")

	validateBucketList(e, bucket, prefix, delim, "", 2, false, []string{"_under1/bar"}, []string{"_under1/baz/"}, "")
}

// validateBucketList checks one page of a listing and returns its NextMarker,
// mirroring upstream's validate_bucket_list. An empty nextMarker means the
// response must not carry one.
//
// delimiters arrive with the rest of the listing tests.
//
//nolint:unparam // delimiter mirrors the upstream helper; callers with other
func validateBucketList(e *fixture.Env, bucket, prefix, delimiter, marker string,
	maxKeys int32, isTruncated bool, checkObjs, checkPrefixes []string, nextMarker string,
) string {
	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(delimiter),
		Marker:    aws.String(marker),
		MaxKeys:   aws.Int32(maxKeys),
		Prefix:    aws.String(prefix),
	})

	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), isTruncated, "is truncated")
	s3util.Equal(e.T, aws.ToString(out.NextMarker), nextMarker, "next marker")
	s3util.EqualStrings(e.T, listKeys(out), checkObjs, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), checkPrefixes, "prefixes")

	return aws.ToString(out.NextMarker)
}

func bucketListDelimiterUnreadable(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "\x0a", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListDelimiterWhitespace(e *fixture.Env) {
	bucket := createObjects(e, "b ar", "b az", "c ab", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(" "),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), " ", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"b ", "c "}, "prefixes")
}

func bucketListEncodingBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo+1/bar", "foo/bar/xyzzy", "quux ab/thud", "asdf+b")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:       aws.String(bucket),
		Delimiter:    aws.String("/"),
		EncodingType: "url",
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"asdf%2Bb"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out),
		[]string{"foo%2B1/", "foo/", "quux%20ab/"}, "prefixes")
}

func bucketListLongName(e *fixture.Env) {
	// Upstream pads the generated name out to 61 characters.
	name := e.NewBucketName()
	for len(name) < 61 {
		name += "a"
	}
	bucket := e.NewBucketNamed(name)

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, len(out.Contents), 0, "object count")
}

func bucketListMany(e *fixture.Env) {
	bucket := createObjects(e, "foo", "bar", "baz")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(2),
	})
	s3util.EqualStrings(e.T, listKeys(out), []string{"bar", "baz"}, "keys")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), true, "is truncated")

	out = listObjects(e, &awss3.ListObjectsInput{
		Bucket:  aws.String(bucket),
		Marker:  aws.String("baz"),
		MaxKeys: aws.Int32(2),
	})
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo"}, "keys")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
}

func bucketListMarkerAfterList(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Marker: aws.String("zzz"),
	})
	s3util.Equal(e.T, aws.ToString(out.Marker), "zzz", "marker")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
}

func bucketListMarkerNotInList(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Marker: aws.String("blah"),
	})
	s3util.Equal(e.T, aws.ToString(out.Marker), "blah", "marker")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo", "quxx"}, "keys")
}

func bucketListMarkerUnreadable(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Marker: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Marker), "\x0a", "marker")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
}

func bucketListMaxkeysInvalid(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	// The SDK would reject a non-numeric MaxKeys before sending, so the
	// value goes on the wire as a raw query parameter instead.
	_, err := e.Client().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
	}, client.WithQuery(map[string]string{"max-keys": "blah"}))
	s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
}

func bucketListMaxkeysNone(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.Equal(e.T, aws.ToInt32(out.MaxKeys), 1000, "max keys")
}

func bucketListMaxkeysOne(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), true, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), keys[0:1], "keys")

	out = listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Marker: aws.String(keys[0]),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), keys[1:], "keys")
}

func bucketListMaxkeysZero(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(0),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
}

func bucketListObjectsAnonymousFail(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.AnonymousClient().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketListPrefixAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String("ba"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "ba", "prefix")
	s3util.EqualStrings(e.T, listKeys(out), []string{"bar", "baz"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String("foo/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "foo/", "prefix")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo/bar", "foo/baz"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixDelimiterAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "bazar", "cab", "foo")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("a"),
		Prefix:    aws.String("ba"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "ba", "prefix")
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "a", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"bar"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"baza"}, "prefixes")
}

func bucketListPrefixDelimiterBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz/xyzzy", "quux/thud", "asdf")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
		Prefix:    aws.String("foo/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "foo/", "prefix")
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeys(out), []string{"foo/bar"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), []string{"foo/baz/"}, "prefixes")
}

func bucketListPrefixDelimiterDelimiterNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/c", "b/a/g", "b/a/r", "g")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("z"),
		Prefix:    aws.String("b"),
	})
	s3util.EqualStrings(e.T, listKeys(out), []string{"b/a/c", "b/a/g", "b/a/r"}, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixDelimiterPrefixDelimiterNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/c", "b/a/g", "b/a/r", "g")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("z"),
		Prefix:    aws.String("y"),
	})
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixDelimiterPrefixNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/r", "b/a/c", "b/a/g", "g")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("d"),
		Prefix:    aws.String("/"),
	})
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixEmpty(e *fixture.Env) {
	keys := []string{"foo/bar", "foo/baz", "quux"}
	bucket := createObjects(e, keys...)

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String(""),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "", "prefix")
	s3util.EqualStrings(e.T, listKeys(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

// bucketListPrefixNone is upstream's test_bucket_list_prefix_none, which is
// identical to prefix_empty: it also passes an empty prefix.
func bucketListPrefixNone(e *fixture.Env) { bucketListPrefixEmpty(e) }

func bucketListPrefixNotExist(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String("d"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "d", "prefix")
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListPrefixUnreadable(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "\x0a", "prefix")
	s3util.EqualStrings(e.T, listKeys(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixes(out), nil, "prefixes")
}

func bucketListReturnData(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo"}
	bucket := createObjects(e, keys...)

	type meta struct {
		etag string
		size int64
	}
	want := map[string]meta{}
	for _, key := range keys {
		head, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		s3util.NoError(e.T, err, "head object "+key)
		want[key] = meta{etag: aws.ToString(head.ETag), size: aws.ToInt64(head.ContentLength)}
	}

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	for _, obj := range out.Contents {
		key := aws.ToString(obj.Key)
		s3util.Equal(e.T, aws.ToString(obj.ETag), want[key].etag, "etag of "+key)
		s3util.Equal(e.T, aws.ToInt64(obj.Size), want[key].size, "size of "+key)
	}
}

func bucketListSpecialPrefix(e *fixture.Env) {
	bucket := createObjects(e, "_bla/1", "_bla/2", "_bla/3", "_bla/4", "abcd")

	out := listObjects(e, &awss3.ListObjectsInput{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, len(listKeys(out)), 5, "object count")

	out = listObjects(e, &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String("_bla/"),
	})
	s3util.Equal(e.T, len(listKeys(out)), 4, "object count under prefix")
}
