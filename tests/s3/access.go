package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func accessTests(b builder) []harness.Test {
	var out []harness.Test

	// Upstream writes these as one test per bucket/object ACL pair. The
	// expectations follow from the two ACLs, so they are derived rather
	// than repeated twelve times -- the names still match upstream exactly.
	for _, bucketACL := range []struct {
		name string
		acl  types.BucketCannedACL
	}{
		{"private", types.BucketCannedACLPrivate},
		{"publicread", types.BucketCannedACLPublicRead},
		{"publicreadwrite", types.BucketCannedACLPublicReadWrite},
	} {
		for _, objectACL := range []struct {
			name string
			acl  types.ObjectCannedACL
		}{
			{"private", types.ObjectCannedACLPrivate},
			{"publicread", types.ObjectCannedACLPublicRead},
			{"publicreadwrite", types.ObjectCannedACLPublicReadWrite},
		} {
			name := "access_bucket_" + bucketACL.name + "_object_" + objectACL.name
			out = append(out, b.add(name, accessBucket(bucketACL.acl, objectACL.acl, false)))

			// The v2 variants exist only for the private bucket, and
			// differ only in which listing API is used.
			if bucketACL.name == "private" {
				v2 := "access_bucket_private_objectv2_" + objectACL.name
				out = append(out, b.add(v2, accessBucket(bucketACL.acl, objectACL.acl, true), markerV2))
			}
		}
	}
	return out
}

// setupAccess creates a bucket with the given ACL holding two objects: key1
// with the given object ACL, key2 with the bucket default. Mirrors upstream's
// _setup_access.
func setupAccess(e *fixture.Env, bucketACL types.BucketCannedACL, objectACL types.ObjectCannedACL) (
	bucket, key1, key2, newKey string,
) {
	bucket = e.NewBucket()
	key1, key2, newKey = "foo", "bar", "new"

	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    bucketACL,
	})
	s3util.NoError(e.T, err, "put bucket acl")

	putObject(e, bucket, key1, "foocontent")
	_, err = e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key1),
		ACL:    objectACL,
	})
	s3util.NoError(e.T, err, "put object acl")

	putObject(e, bucket, key2, "barcontent")
	return bucket, key1, key2, newKey
}

// accessBucket checks what the alt user may do given a bucket and object ACL.
//
// Reads of the ACLed object follow the object ACL; reads of the default object
// and listing the bucket follow the bucket ACL; writes follow the bucket ACL,
// since an object ACL cannot grant the right to create a key.
func accessBucket(bucketACL types.BucketCannedACL, objectACL types.ObjectCannedACL, useV2 bool) func(*fixture.Env) {
	objectReadable := objectACL == types.ObjectCannedACLPublicRead ||
		objectACL == types.ObjectCannedACLPublicReadWrite
	bucketReadable := bucketACL == types.BucketCannedACLPublicRead ||
		bucketACL == types.BucketCannedACLPublicReadWrite
	bucketWritable := bucketACL == types.BucketCannedACLPublicReadWrite

	return func(e *fixture.Env) {
		bucket, key1, key2, newKey := setupAccess(e, bucketACL, objectACL)

		check := func(err error, allowed bool, what string) {
			if allowed {
				s3util.NoError(e.T, err, what)
				return
			}
			accessDenied(e, err, what)
		}

		check(altGetKey(e, bucket, key1), objectReadable, "alt read of the acled object")
		check(altGetKey(e, bucket, key2), bucketReadable, "alt read of the default object")
		check(altList(e, bucket, useV2), bucketReadable, "alt list of the bucket")

		check(altPutKey(e, bucket, key1), bucketWritable, "alt write of the acled object")
		check(altPutKey(e, bucket, key2), bucketWritable, "alt write of the default object")
		check(altPutKey(e, bucket, newKey), bucketWritable, "alt write of a new key")
	}
}

func altGetKey(e *fixture.Env, bucket, key string) error {
	out, err := e.AltClient().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	_ = out.Body.Close()
	return nil
}

func altPutKey(e *fixture.Env, bucket, key string) error {
	_, err := e.AltClient().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   readerOf("overwrite"),
	})
	return err
}

func altList(e *fixture.Env, bucket string, useV2 bool) error {
	if useV2 {
		_, err := e.AltClient().ListObjectsV2(e.Ctx(), &awss3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		return err
	}
	_, err := e.AltClient().ListObjects(e.Ctx(), &awss3.ListObjectsInput{
		Bucket: aws.String(bucket),
	})
	return err
}
