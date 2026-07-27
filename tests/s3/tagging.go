package s3

import (
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerTagging is on every test in this file, matching upstream.
const markerTagging = "tagging"

func taggingTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("get_obj_tagging", getObjTagging, markerTagging, "fails_on_dbstore"),
		b.add("put_delete_tags", putDeleteTags, markerTagging, "fails_on_dbstore"),
		b.add("put_max_tags", putMaxTags, markerTagging, "fails_on_dbstore"),
		b.add("put_excess_tags", putExcessTags, markerTagging),
		b.add("put_max_kvsize_tags", putMaxKVSizeTags, markerTagging),
		b.add("put_excess_key_tags", putExcessKeyTags, markerTagging),
		b.add("put_excess_val_tags", putExcessValTags, markerTagging),
		b.add("put_modify_tags", putModifyTags, markerTagging, "fails_on_dbstore"),
	}
}

// simpleTagset builds count tags whose key and value are both the index,
// mirroring upstream's _create_simple_tagset.
func simpleTagset(count int) []types.Tag {
	tags := make([]types.Tag, 0, count)
	for i := range count {
		s := strconv.Itoa(i)
		tags = append(tags, types.Tag{Key: aws.String(s), Value: aws.String(s)})
	}
	return tags
}

// sizedTagset builds count tags with keys and values of the given lengths, for
// the tests that probe the size limits.
func sizedTagset(count, keyLen, valLen int) []types.Tag {
	tags := make([]types.Tag, 0, count)
	for range count {
		tags = append(tags, types.Tag{
			Key:   aws.String(s3util.RandomString(keyLen)),
			Value: aws.String(s3util.RandomString(valLen)),
		})
	}
	return tags
}

func putObjectTagging(e *fixture.Env, bucket, key string, tags []types.Tag) error {
	_, err := e.Client().PutObjectTagging(e.Ctx(), &awss3.PutObjectTaggingInput{
		Bucket:  aws.String(bucket),
		Key:     aws.String(key),
		Tagging: &types.Tagging{TagSet: tags},
	})
	return err
}

func getObjectTagging(e *fixture.Env, bucket, key string) []types.Tag {
	out, err := e.Client().GetObjectTagging(e.Ctx(), &awss3.GetObjectTaggingInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "get object tagging")
	return out.TagSet
}

// equalTags compares two tag sets by order, as upstream does.
func equalTags(e *fixture.Env, got, want []types.Tag, what string) {
	if len(got) != len(want) {
		e.T.Errorf("%s has %d tags, want %d", what, len(got), len(want))
		return
	}
	for i := range want {
		s3util.Equal(e.T, aws.ToString(got[i].Key), aws.ToString(want[i].Key), what+" key")
		s3util.Equal(e.T, aws.ToString(got[i].Value), aws.ToString(want[i].Value), what+" value")
	}
}

func getObjTagging(e *fixture.Env) {
	const key = "testputtags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	tags := simpleTagset(2)
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, tags), "put object tagging")
	equalTags(e, getObjectTagging(e, bucket, key), tags, "tag set")
}

func putDeleteTags(e *fixture.Env) {
	const key = "testputmodifytags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	tags := simpleTagset(2)
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, tags), "put object tagging")
	equalTags(e, getObjectTagging(e, bucket, key), tags, "tag set")

	out, err := e.Client().DeleteObjectTagging(e.Ctx(), &awss3.DeleteObjectTaggingInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "delete object tagging")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 204, "status")

	s3util.Equal(e.T, len(getObjectTagging(e, bucket, key)), 0, "tag count after delete")
}

func putMaxTags(e *fixture.Env) {
	const key = "testputmaxtags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	tags := simpleTagset(10)
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, tags), "put object tagging")
	equalTags(e, getObjectTagging(e, bucket, key), tags, "tag set")
}

func putExcessTags(e *fixture.Env) {
	const key = "testputmaxtags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	// Eleven tags is one past the limit.
	err := putObjectTagging(e, bucket, key, simpleTagset(11))
	s3util.ErrorIs(e.T, err, 400, "InvalidTag")
	s3util.Equal(e.T, len(getObjectTagging(e, bucket, key)), 0, "tag count")
}

func putMaxKVSizeTags(e *fixture.Env) {
	const key = "testputmaxkeysize"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	// 128-byte keys and 256-byte values are the largest allowed.
	tags := sizedTagset(10, 128, 256)
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, tags), "put object tagging")

	// Membership, not order: upstream checks each returned tag is one that
	// was sent, and this is the only tagging test whose keys are random
	// rather than "0".."9". Comparing positions here would fail any server
	// that returns tags sorted -- which go-faster/fs does -- on an
	// assertion upstream never makes.
	containsTags(e, getObjectTagging(e, bucket, key), tags)
}

// containsTags asserts every returned tag was among those sent, mirroring
// upstream's membership check.
func containsTags(e *fixture.Env, got, sent []types.Tag) {
	s3util.Equal(e.T, len(got), len(sent), "tag count")
	for _, g := range got {
		found := false
		for _, s := range sent {
			if aws.ToString(g.Key) == aws.ToString(s.Key) &&
				aws.ToString(g.Value) == aws.ToString(s.Value) {
				found = true
				break
			}
		}
		if !found {
			e.T.Errorf("returned tag %q was never sent", aws.ToString(g.Key))
		}
	}
}

func putExcessKeyTags(e *fixture.Env) {
	const key = "testputexcesskeytags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	err := putObjectTagging(e, bucket, key, sizedTagset(10, 129, 256))
	s3util.ErrorIs(e.T, err, 400, "InvalidTag")
	s3util.Equal(e.T, len(getObjectTagging(e, bucket, key)), 0, "tag count")
}

func putExcessValTags(e *fixture.Env) {
	const key = "testputexcesskeytags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	err := putObjectTagging(e, bucket, key, sizedTagset(10, 128, 257))
	s3util.ErrorIs(e.T, err, 400, "InvalidTag")
	s3util.Equal(e.T, len(getObjectTagging(e, bucket, key)), 0, "tag count")
}

func putModifyTags(e *fixture.Env) {
	const key = "testputmodifytags"
	bucket := createKeyWithRandomContent(e, key, 7*1024*1024, "")

	first := []types.Tag{
		{Key: aws.String("key"), Value: aws.String("val")},
		{Key: aws.String("key2"), Value: aws.String("val2")},
	}
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, first), "put object tagging")
	equalTags(e, getObjectTagging(e, bucket, key), first, "tag set")

	// A second put replaces the set rather than merging into it.
	second := []types.Tag{{Key: aws.String("key3"), Value: aws.String("val3")}}
	s3util.NoError(e.T, putObjectTagging(e, bucket, key, second), "put object tagging")
	equalTags(e, getObjectTagging(e, bucket, key), second, "tag set after modify")
}
