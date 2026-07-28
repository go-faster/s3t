package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerV2 is on every test in this file, matching upstream.
const markerV2 = "list_objects_v2"

func listingV2Tests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_listv2_both_continuationtoken_startafter", bucketListv2BothContinuationtokenStartafter, markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_continuationtoken_empty", bucketListv2ContinuationtokenEmpty, markerV2),
		b.add("bucket_listv2_delimiter_basic", bucketListv2DelimiterBasic, markerV2),
		b.add("bucket_listv2_delimiter_prefix_ends_with_delimiter",
			bucketListv2DelimiterPrefixEndsWithDelimiter, markerV2),
		b.add("bucket_listv2_objects_anonymous", bucketListv2ObjectsAnonymous, markerV2),
		b.add("bucket_listv2_unordered", bucketListv2Unordered, "fails_on_aws", markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_continuationtoken", bucketListv2Continuationtoken, markerV2),
		b.add("bucket_listv2_delimiter_alt", bucketListv2DelimiterAlt, markerV2),
		b.add("bucket_listv2_delimiter_dot", bucketListv2DelimiterDot, markerV2),
		b.add("bucket_listv2_delimiter_empty", bucketListv2DelimiterEmpty, markerV2),
		b.add("bucket_listv2_delimiter_none", bucketListv2DelimiterNone, markerV2),
		b.add("bucket_listv2_delimiter_not_exist", bucketListv2DelimiterNotExist, markerV2),
		b.add("bucket_listv2_delimiter_percentage", bucketListv2DelimiterPercentage, markerV2),
		b.add("bucket_listv2_delimiter_prefix", bucketListv2DelimiterPrefix, markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_delimiter_prefix_underscore", bucketListv2DelimiterPrefixUnderscore, markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_delimiter_unreadable", bucketListv2DelimiterUnreadable, markerV2),
		b.add("bucket_listv2_delimiter_whitespace", bucketListv2DelimiterWhitespace, markerV2),
		b.add("bucket_listv2_encoding_basic", bucketListv2EncodingBasic, markerV2),
		b.add("bucket_listv2_fetchowner_defaultempty", bucketListv2FetchownerDefaultempty, markerV2),
		b.add("bucket_listv2_fetchowner_empty", bucketListv2FetchownerEmpty, markerV2),
		b.add("bucket_listv2_fetchowner_notempty", bucketListv2FetchownerNotempty, markerV2),
		b.add("bucket_listv2_many", bucketListv2Many, markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_maxkeys_none", bucketListv2MaxkeysNone, markerV2),
		b.add("bucket_listv2_maxkeys_one", bucketListv2MaxkeysOne, markerV2, "fails_on_dbstore"),
		b.add("bucket_listv2_maxkeys_zero", bucketListv2MaxkeysZero, markerV2),
		b.add("bucket_listv2_objects_anonymous_fail", bucketListv2ObjectsAnonymousFail, markerV2),
		b.add("bucket_listv2_prefix_alt", bucketListv2PrefixAlt, markerV2),
		b.add("bucket_listv2_prefix_basic", bucketListv2PrefixBasic, markerV2),
		b.add("bucket_listv2_prefix_delimiter_alt", bucketListv2PrefixDelimiterAlt, markerV2),
		b.add("bucket_listv2_prefix_delimiter_basic", bucketListv2PrefixDelimiterBasic, markerV2),
		b.add("bucket_listv2_prefix_delimiter_delimiter_not_exist", bucketListv2PrefixDelimiterDelimiterNotExist, markerV2),
		b.add("bucket_listv2_prefix_delimiter_prefix_delimiter_not_exist", bucketListv2PrefixDelimiterPrefixDelimiterNotExist, markerV2),
		b.add("bucket_listv2_prefix_delimiter_prefix_not_exist", bucketListv2PrefixDelimiterPrefixNotExist, markerV2),
		b.add("bucket_listv2_prefix_empty", bucketListv2PrefixEmpty, markerV2),
		b.add("bucket_listv2_prefix_none", bucketListv2PrefixNone, markerV2),
		b.add("bucket_listv2_prefix_not_exist", bucketListv2PrefixNotExist, markerV2),
		b.add("bucket_listv2_prefix_unreadable", bucketListv2PrefixUnreadable, markerV2),
		b.add("bucket_listv2_startafter_after_list", bucketListv2StartafterAfterList, markerV2),
		b.add("bucket_listv2_startafter_not_in_list", bucketListv2StartafterNotInList, markerV2),
		b.add("bucket_listv2_startafter_unreadable", bucketListv2StartafterUnreadable, markerV2),
	}
}

func listKeysV2(out *awss3.ListObjectsV2Output) []string {
	keys := make([]string, 0, len(out.Contents))
	for _, o := range out.Contents {
		keys = append(keys, aws.ToString(o.Key))
	}
	return keys
}

func listPrefixesV2(out *awss3.ListObjectsV2Output) []string {
	prefixes := make([]string, 0, len(out.CommonPrefixes))
	for _, p := range out.CommonPrefixes {
		prefixes = append(prefixes, aws.ToString(p.Prefix))
	}
	return prefixes
}

func listV2(e *fixture.Env, in *awss3.ListObjectsV2Input) *awss3.ListObjectsV2Output {
	out, err := e.Client().ListObjectsV2(e.Ctx(), in)
	s3util.NoError(e.T, err, "list objects v2")
	return out
}

func bucketListv2BothContinuationtokenStartafter(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	first := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String("bar"),
		MaxKeys:    aws.Int32(1),
	})
	token := aws.ToString(first.NextContinuationToken)

	second := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:            aws.String(bucket),
		StartAfter:        aws.String("bar"),
		ContinuationToken: aws.String(token),
	})
	s3util.Equal(e.T, aws.ToString(second.ContinuationToken), token, "continuation token")
	s3util.Equal(e.T, aws.ToString(second.StartAfter), "bar", "start after")
	s3util.Equal(e.T, aws.ToBool(second.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(second), []string{"foo", "quxx"}, "keys")
}

func bucketListv2Continuationtoken(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	first := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1),
	})
	token := aws.ToString(first.NextContinuationToken)

	second := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:            aws.String(bucket),
		ContinuationToken: aws.String(token),
	})
	s3util.Equal(e.T, aws.ToString(second.ContinuationToken), token, "continuation token")
	s3util.Equal(e.T, aws.ToBool(second.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(second), []string{"baz", "foo", "quxx"}, "keys")
}

func bucketListv2DelimiterAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "cab", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "a", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"ba", "ca"}, "prefixes")
}

func bucketListv2DelimiterDot(e *fixture.Env) {
	bucket := createObjects(e, "b.ar", "b.az", "c.ab", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("."),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), ".", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"b.", "c."}, "prefixes")
}

func bucketListv2DelimiterEmpty(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(""),
	})
	s3util.Equal(e.T, out.Delimiter == nil, true, "delimiter absent")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2DelimiterNone(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, out.Delimiter == nil, true, "delimiter absent")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2DelimiterNotExist(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2DelimiterPercentage(e *fixture.Env) {
	bucket := createObjects(e, "b%ar", "b%az", "c%ab", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("%"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "%", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"b%", "c%"}, "prefixes")
}

func bucketListv2DelimiterPrefix(e *fixture.Env) {
	bucket := createObjects(e, "asdf", "boo/bar", "boo/baz/xyzzy", "cquux/thud", "cquux/bla")

	const delim = "/"
	prefix := ""

	token := validateBucketListV2(e, bucket, prefix, delim, "", 1, true, []string{"asdf"}, nil, false)
	token = validateBucketListV2(e, bucket, prefix, delim, token, 1, true, nil, []string{"boo/"}, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 1, false, nil, []string{"cquux/"}, true)

	token = validateBucketListV2(e, bucket, prefix, delim, "", 2, true, []string{"asdf"}, []string{"boo/"}, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 2, false, nil, []string{"cquux/"}, true)

	prefix = "boo/"

	token = validateBucketListV2(e, bucket, prefix, delim, "", 1, true, []string{"boo/bar"}, nil, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 1, false, nil, []string{"boo/baz/"}, true)

	validateBucketListV2(e, bucket, prefix, delim, "", 2, false, []string{"boo/bar"}, []string{"boo/baz/"}, true)
}

func bucketListv2DelimiterPrefixUnderscore(e *fixture.Env) {
	bucket := createObjects(e, "_obj1_", "_under1/bar", "_under1/baz/xyzzy", "_under2/thud", "_under2/bla")

	const delim = "/"
	prefix := ""

	token := validateBucketListV2(e, bucket, prefix, delim, "", 1, true, []string{"_obj1_"}, nil, false)
	token = validateBucketListV2(e, bucket, prefix, delim, token, 1, true, nil, []string{"_under1/"}, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 1, false, nil, []string{"_under2/"}, true)

	token = validateBucketListV2(e, bucket, prefix, delim, "", 2, true, []string{"_obj1_"}, []string{"_under1/"}, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 2, false, nil, []string{"_under2/"}, true)

	prefix = "_under1/"

	token = validateBucketListV2(e, bucket, prefix, delim, "", 1, true, []string{"_under1/bar"}, nil, false)
	validateBucketListV2(e, bucket, prefix, delim, token, 1, false, nil, []string{"_under1/baz/"}, true)

	validateBucketListV2(e, bucket, prefix, delim, "", 2, false, []string{"_under1/bar"}, []string{"_under1/baz/"}, true)
}

// validateBucketListV2 checks one page and returns its continuation token,
// mirroring upstream's validate_bucket_listv2. An empty token means the first
// page, where upstream sends StartAfter=” instead.
//
//nolint:unparam // delimiter mirrors the upstream helper signature.
func validateBucketListV2(e *fixture.Env, bucket, prefix, delimiter, token string,
	maxKeys int32, isTruncated bool, checkObjs, checkPrefixes []string, last bool,
) string {
	in := &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(delimiter),
		MaxKeys:   aws.Int32(maxKeys),
		Prefix:    aws.String(prefix),
	}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	} else {
		in.StartAfter = aws.String("")
	}

	out := listV2(e, in)
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), isTruncated, "is truncated")
	if last {
		s3util.Equal(e.T, out.NextContinuationToken == nil, true, "next continuation token absent")
	}
	s3util.EqualStrings(e.T, listKeysV2(out), checkObjs, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), checkPrefixes, "prefixes")

	return aws.ToString(out.NextContinuationToken)
}

func bucketListv2DelimiterUnreadable(e *fixture.Env) {
	keys := []string{"bar", "baz", "cab", "foo"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "\x0a", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2DelimiterWhitespace(e *fixture.Env) {
	bucket := createObjects(e, "b ar", "b az", "c ab", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String(" "),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), " ", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"b ", "c "}, "prefixes")
}

func bucketListv2EncodingBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo+1/bar", "foo/bar/xyzzy", "quux ab/thud", "asdf+b")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:       aws.String(bucket),
		Delimiter:    aws.String("/"),
		EncodingType: "url",
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"asdf%2Bb"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out),
		[]string{"foo%2B1/", "foo/", "quux%20ab/"}, "prefixes")
}

func bucketListv2FetchownerDefaultempty(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	s3util.EqualNow(e.T, len(out.Contents) > 0, true, "listing is non-empty")
	s3util.Equal(e.T, out.Contents[0].Owner == nil, true, "owner absent")
}

func bucketListv2FetchownerEmpty(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		FetchOwner: aws.Bool(false),
	})
	s3util.EqualNow(e.T, len(out.Contents) > 0, true, "listing is non-empty")
	s3util.Equal(e.T, out.Contents[0].Owner == nil, true, "owner absent")
}

func bucketListv2FetchownerNotempty(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		FetchOwner: aws.Bool(true),
	})
	s3util.EqualNow(e.T, len(out.Contents) > 0, true, "listing is non-empty")
	s3util.Equal(e.T, out.Contents[0].Owner != nil, true, "owner present")
}

func bucketListv2Many(e *fixture.Env) {
	bucket := createObjects(e, "foo", "bar", "baz")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(2),
	})
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"bar", "baz"}, "keys")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), true, "is truncated")

	out = listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String("baz"),
		MaxKeys:    aws.Int32(2),
	})
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo"}, "keys")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
}

func bucketListv2MaxkeysNone(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.Equal(e.T, aws.ToInt32(out.MaxKeys), 1000, "max keys")
}

func bucketListv2MaxkeysOne(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), true, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), keys[0:1], "keys")

	out = listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String(keys[0]),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), keys[1:], "keys")
}

func bucketListv2MaxkeysZero(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(0),
	})
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
}

func bucketListv2ObjectsAnonymousFail(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.AnonymousClient().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func bucketListv2PrefixAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("ba"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "ba", "prefix")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"bar", "baz"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("foo/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "foo/", "prefix")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo/bar", "foo/baz"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixDelimiterAlt(e *fixture.Env) {
	bucket := createObjects(e, "bar", "bazar", "cab", "foo")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("a"),
		Prefix:    aws.String("ba"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "ba", "prefix")
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "a", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"bar"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"baza"}, "prefixes")
}

func bucketListv2PrefixDelimiterBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz/xyzzy", "quux/thud", "asdf")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
		Prefix:    aws.String("foo/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "foo/", "prefix")
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo/bar"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), []string{"foo/baz/"}, "prefixes")
}

func bucketListv2PrefixDelimiterDelimiterNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/c", "b/a/g", "b/a/r", "g")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("z"),
		Prefix:    aws.String("b"),
	})
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"b/a/c", "b/a/g", "b/a/r"}, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixDelimiterPrefixDelimiterNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/c", "b/a/g", "b/a/r", "g")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("z"),
		Prefix:    aws.String("y"),
	})
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixDelimiterPrefixNotExist(e *fixture.Env) {
	bucket := createObjects(e, "b/a/r", "b/a/c", "b/a/g", "g")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("d"),
		Prefix:    aws.String("/"),
	})
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixEmpty(e *fixture.Env) {
	keys := []string{"foo/bar", "foo/baz", "quux"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(""),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "", "prefix")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

// bucketListv2PrefixNone mirrors upstream, where prefix_none is identical to
// prefix_empty.
func bucketListv2PrefixNone(e *fixture.Env) { bucketListv2PrefixEmpty(e) }

func bucketListv2PrefixNotExist(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("d"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "d", "prefix")
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2PrefixUnreadable(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/baz", "quux")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.Prefix), "\x0a", "prefix")
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
	s3util.EqualStrings(e.T, listPrefixesV2(out), nil, "prefixes")
}

func bucketListv2StartafterAfterList(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String("zzz"),
	})
	s3util.Equal(e.T, aws.ToString(out.StartAfter), "zzz", "start after")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), nil, "keys")
}

func bucketListv2StartafterNotInList(e *fixture.Env) {
	bucket := createObjects(e, "bar", "baz", "foo", "quxx")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String("blah"),
	})
	s3util.Equal(e.T, aws.ToString(out.StartAfter), "blah", "start after")
	s3util.EqualStrings(e.T, listKeysV2(out), []string{"foo", "quxx"}, "keys")
}

func bucketListv2StartafterUnreadable(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		StartAfter: aws.String("\x0a"),
	})
	s3util.Equal(e.T, aws.ToString(out.StartAfter), "\x0a", "start after")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
}

func bucketListv2ContinuationtokenEmpty(e *fixture.Env) {
	keys := []string{"bar", "baz", "foo", "quxx"}
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:            aws.String(bucket),
		ContinuationToken: aws.String(""),
	})
	s3util.Equal(e.T, mustField(e, out.ContinuationToken, "ContinuationToken"), "", "continuation token")
	s3util.Equal(e.T, aws.ToBool(out.IsTruncated), false, "is truncated")
	s3util.EqualStrings(e.T, listKeysV2(out), keys, "keys")
}

func bucketListv2DelimiterBasic(e *fixture.Env) {
	bucket := createObjects(e, "foo/bar", "foo/bar/xyzzy", "quux/thud", "asdf")

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})
	s3util.Equal(e.T, aws.ToString(out.Delimiter), "/", "delimiter")
	keys, prefixes := listKeysV2(out), listPrefixesV2(out)
	s3util.EqualStrings(e.T, keys, []string{"asdf"}, "keys")
	s3util.EqualStrings(e.T, prefixes, []string{"foo/", "quux/"}, "prefixes")
	s3util.Equal(e.T, int(aws.ToInt32(out.KeyCount)), len(keys)+len(prefixes), "key count")
}

func bucketListv2DelimiterPrefixEndsWithDelimiter(e *fixture.Env) {
	bucket := createObjects(e, "asdf/")
	validateBucketListV2(e, bucket, "asdf/", "/", "", 1000, false, []string{"asdf/"}, nil, true)
}

func bucketListv2ObjectsAnonymous(e *fixture.Env) {
	bucket := e.NewBucket()
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    types.BucketCannedACLPublicRead,
	})
	s3util.NoError(e.T, err, "put bucket acl")

	_, err = e.AnonymousClient().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list objects anonymously")
}

// bucketListv2Unordered mirrors bucketListUnordered, including the detail that
// the v2 listings do not actually carry allow-unordered: upstream registers
// the hook on before-call.s3.ListObjects, an event a list_objects_v2 call
// never emits, so only the final ListObjects gets the parameter.
func bucketListv2Unordered(e *fixture.Env) {
	keys := unorderedKeys()
	bucket := createObjects(e, keys...)

	out := listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1000),
	})
	s3util.Equal(e.T, len(listKeysV2(out)), len(keys), "key count")

	out = listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1000),
		Prefix:  aws.String("abc/"),
	})
	s3util.Equal(e.T, len(listKeysV2(out)), 5, "key count under abc/")

	out = listV2(e, &awss3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(6),
	})
	first := listKeysV2(out)
	s3util.EqualNow(e.T, len(first), 6, "first page size")

	out = listV2(e, &awss3.ListObjectsV2Input{
		Bucket:     aws.String(bucket),
		MaxKeys:    aws.Int32(6),
		StartAfter: aws.String(first[len(first)-1]),
	})
	second := listKeysV2(out)
	s3util.Equal(e.T, len(second), 6, "second page size")
	s3util.Equal(e.T, overlaps(first, second), false, "pages overlap")

	_, err := e.Client().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	}, allowUnordered())
	s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
}
