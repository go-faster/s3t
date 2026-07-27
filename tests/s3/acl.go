package s3

import (
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func aclTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_acl", objectACL, "fails_on_aws"),
		b.add("object_acl_default", objectACLDefault),
		b.add("object_acl_full_control_verify_owner", objectACLFullControlVerifyOwner, "fails_on_aws"),
		b.add("object_acl_full_control_verify_attributes", objectACLFullControlVerifyAttributes),
		b.add("object_raw_get", objectRawGet),
		b.add("object_raw_get_bucket_gone", objectRawGetBucketGone),
		b.add("object_raw_get_object_gone", objectRawGetObjectGone),
		b.add("object_raw_get_bucket_acl", objectRawGetBucketACL),
		b.add("object_raw_authenticated", objectRawAuthenticated),
		b.add("object_raw_authenticated_bucket_acl", objectRawAuthenticatedBucketACL),
		b.add("object_raw_authenticated_object_acl", objectRawAuthenticatedObjectACL),
		b.add("object_raw_authenticated_bucket_gone", objectRawAuthenticatedBucketGone),
		b.add("object_raw_authenticated_object_gone", objectRawAuthenticatedObjectGone),
		b.add("object_anon_put", objectAnonPut),
		b.add("object_anon_put_write_access", objectAnonPutWriteAccess),
		b.add("object_put_authenticated", objectPutAuthenticated),
		b.add("object_delete_key_bucket_gone", objectDeleteKeyBucketGone),
	}
}

// setupBucketObjectACL creates a bucket and a "foo" object with the given
// canned ACLs, mirroring upstream's _setup_bucket_object_acl.
func setupBucketObjectACL(e *fixture.Env, bucketACL types.BucketCannedACL, objectACL types.ObjectCannedACL) string {
	name := e.NewBucketName()
	_, err := e.Client().CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket: aws.String(name),
		ACL:    bucketACL,
	})
	s3util.NoError(e.T, err, "create bucket")
	e.T.Cleanup(func() { e.Nuke(e.Client(), name) })

	_, err = e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(name),
		Key:    aws.String("foo"),
		ACL:    objectACL,
	})
	s3util.NoError(e.T, err, "put object")
	return name
}

// setupBucketACL creates a bucket with a canned ACL, mirroring upstream's
// _setup_bucket_acl.
func setupBucketACL(e *fixture.Env, bucketACL types.BucketCannedACL) string {
	name := e.NewBucketName()
	_, err := e.Client().CreateBucket(e.Ctx(), &awss3.CreateBucketInput{
		Bucket: aws.String(name),
		ACL:    bucketACL,
	})
	s3util.NoError(e.T, err, "create bucket")
	e.T.Cleanup(func() { e.Nuke(e.Client(), name) })
	return name
}

// wantGrant is one expected entry of an ACL.
type wantGrant struct {
	permission  types.Permission
	id          string
	displayName string
	grantType   types.Type
}

// checkGrants compares an ACL against the expected grants in any order,
// mirroring upstream's check_grants.
func checkGrants(e *fixture.Env, got []types.Grant, want []wantGrant) {
	if len(got) != len(want) {
		e.T.Fatalf("got %d grants, want %d", len(got), len(want))
	}

	sort.Slice(got, func(i, j int) bool {
		return aws.ToString(got[i].Grantee.DisplayName) < aws.ToString(got[j].Grantee.DisplayName)
	})
	sort.Slice(want, func(i, j int) bool { return want[i].displayName < want[j].displayName })

	for i, w := range want {
		g := got[i]
		s3util.Equal(e.T, g.Permission, w.permission, "permission")
		s3util.Equal(e.T, aws.ToString(g.Grantee.ID), w.id, "grantee id")
		s3util.Equal(e.T, aws.ToString(g.Grantee.DisplayName), w.displayName, "grantee display name")
		s3util.Equal(e.T, g.Grantee.Type, w.grantType, "grantee type")
	}
}

// aclKey is the key every ACL and raw-access test uses, as upstream does.
const aclKey = "foo"

func getObjectACL(e *fixture.Env, bucket string) *awss3.GetObjectAclOutput {
	out, err := e.Client().GetObjectAcl(e.Ctx(), &awss3.GetObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(aclKey),
	})
	s3util.NoError(e.T, err, "get object acl")
	return out
}

func objectACL(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	// Read the ACL back, change the permission, and write it again.
	acl := getObjectACL(e, bucket)
	s3util.EqualNow(e.T, len(acl.Grants) > 0, true, "acl has grants")
	acl.Grants[0].Permission = types.PermissionFullControl

	_, err := e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner:  acl.Owner,
			Grants: acl.Grants,
		},
	})
	s3util.NoError(e.T, err, "put object acl")

	checkGrants(e, getObjectACL(e, bucket).Grants, []wantGrant{{
		permission:  types.PermissionFullControl,
		id:          e.Cfg.Main.UserID,
		displayName: e.Cfg.Main.DisplayName,
		grantType:   types.TypeCanonicalUser,
	}})
}

func objectACLDefault(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	checkGrants(e, getObjectACL(e, bucket).Grants, []wantGrant{{
		permission:  types.PermissionFullControl,
		id:          e.Cfg.Main.UserID,
		displayName: e.Cfg.Main.DisplayName,
		grantType:   types.TypeCanonicalUser,
	}})
}

func objectACLFullControlVerifyOwner(e *fixture.Env) {
	bucket := setupBucketACL(e, types.BucketCannedACLPublicReadWrite)
	putObject(e, bucket, "foo", "bar")

	owner := &types.Owner{
		DisplayName: aws.String(e.Cfg.Main.DisplayName),
		ID:          aws.String(e.Cfg.Main.UserID),
	}
	altGrantee := &types.Grantee{
		ID:   aws.String(e.Cfg.Alt.UserID),
		Type: types.TypeCanonicalUser,
	}

	// Hand full control to the alt user.
	_, err := e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner:  owner,
			Grants: []types.Grant{{Grantee: altGrantee, Permission: types.PermissionFullControl}},
		},
	})
	s3util.NoError(e.T, err, "put object acl as owner")

	// The alt user can now rewrite the ACL, but ownership does not move.
	_, err = e.AltClient().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner:  owner,
			Grants: []types.Grant{{Grantee: altGrantee, Permission: types.PermissionReadAcp}},
		},
	})
	s3util.NoError(e.T, err, "put object acl as alt user")

	out, err := e.AltClient().GetObjectAcl(e.Ctx(), &awss3.GetObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object acl as alt user")
	s3util.Equal(e.T, aws.ToString(out.Owner.ID), e.Cfg.Main.UserID, "owner id")
}

func objectACLFullControlVerifyAttributes(e *fixture.Env) {
	bucket := setupBucketACL(e, types.BucketCannedACLPublicReadWrite)

	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   readerOf("bar"),
	}, client.WithHeaders(map[string]string{"x-amz-foo": "bar"}))
	s3util.NoError(e.T, err, "put object")

	before, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object")
	contentType, etag := aws.ToString(before.ContentType), aws.ToString(before.ETag)
	_ = before.Body.Close()

	// Adding a grant must not disturb the object's own attributes.
	acl := getObjectACL(e, bucket)
	grants := append(acl.Grants, types.Grant{
		Grantee:    &types.Grantee{ID: aws.String(e.Cfg.Alt.UserID), Type: types.TypeCanonicalUser},
		Permission: types.PermissionFullControl,
	})
	_, err = e.Client().PutObjectAcl(e.Ctx(), &awss3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner: &types.Owner{
				DisplayName: aws.String(e.Cfg.Main.DisplayName),
				ID:          aws.String(e.Cfg.Main.UserID),
			},
			Grants: grants,
		},
	})
	s3util.NoError(e.T, err, "put object acl")

	after, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = after.Body.Close() }()
	s3util.Equal(e.T, aws.ToString(after.ContentType), contentType, "content type")
	s3util.Equal(e.T, aws.ToString(after.ETag), etag, "etag")
}

// anonGet reads a key with no credentials and returns the HTTP status.
func anonGet(e *fixture.Env, bucket string) (int, error) {
	out, err := e.AnonymousClient().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(aclKey),
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = out.Body.Close() }()
	return client.Status(out.ResultMetadata), nil
}

func objectRawGet(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)

	status, err := anonGet(e, bucket)
	s3util.NoError(e.T, err, "anonymous get")
	s3util.Equal(e.T, status, 200, "status")
}

func objectRawGetBucketGone(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)

	deleteObject(e, bucket)
	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{Bucket: aws.String(bucket)})
	s3util.NoError(e.T, err, "delete bucket")

	_, err = anonGet(e, bucket)
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func objectRawGetObjectGone(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)
	deleteObject(e, bucket)

	_, err := anonGet(e, bucket)
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
}

func objectRawGetBucketACL(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPrivate, types.ObjectCannedACLPublicRead)

	// The object grant is enough even though the bucket is private.
	status, err := anonGet(e, bucket)
	s3util.NoError(e.T, err, "anonymous get")
	s3util.Equal(e.T, status, 200, "status")
}

// authGet reads a key as the main user and returns the HTTP status.
func authGet(e *fixture.Env, bucket string) (int, error) {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(aclKey),
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = out.Body.Close() }()
	return client.Status(out.ResultMetadata), nil
}

func objectRawAuthenticated(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)

	status, err := authGet(e, bucket)
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, status, 200, "status")
}

func objectRawAuthenticatedBucketACL(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPrivate, types.ObjectCannedACLPublicRead)

	status, err := authGet(e, bucket)
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, status, 200, "status")
}

func objectRawAuthenticatedObjectACL(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPrivate)

	status, err := authGet(e, bucket)
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, status, 200, "status")
}

func objectRawAuthenticatedBucketGone(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)

	deleteObject(e, bucket)
	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{Bucket: aws.String(bucket)})
	s3util.NoError(e.T, err, "delete bucket")

	_, err = authGet(e, bucket)
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func objectRawAuthenticatedObjectGone(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)
	deleteObject(e, bucket)

	_, err := authGet(e, bucket)
	s3util.ErrorIs(e.T, err, 404, "NoSuchKey")
}

func objectAnonPut(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "")

	_, err := e.AnonymousClient().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   readerOf("foo"),
	})
	s3util.ErrorIs(e.T, err, 403, "AccessDenied")
}

func objectAnonPutWriteAccess(e *fixture.Env) {
	bucket := setupBucketACL(e, types.BucketCannedACLPublicReadWrite)
	putObject(e, bucket, "foo", "")

	out, err := e.AnonymousClient().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
		Body:   readerOf("foo"),
	})
	s3util.NoError(e.T, err, "anonymous put")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}

func objectPutAuthenticated(e *fixture.Env) {
	bucket := e.NewBucket()

	out := putObject(e, bucket, "foo", "foo")
	s3util.Equal(e.T, client.Status(out.ResultMetadata), 200, "status")
}

func objectDeleteKeyBucketGone(e *fixture.Env) {
	bucket := setupBucketObjectACL(e, types.BucketCannedACLPublicRead, types.ObjectCannedACLPublicRead)

	deleteObject(e, bucket)
	_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{Bucket: aws.String(bucket)})
	s3util.NoError(e.T, err, "delete bucket")

	_, err = e.AnonymousClient().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func deleteObject(e *fixture.Env, bucket string) {
	_, err := e.Client().DeleteObject(e.Ctx(), &awss3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(aclKey),
	})
	s3util.NoError(e.T, err, "delete object")
}
