package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// The group URIs S3 uses for canned ACLs.
const (
	uriAllUsers           = "http://acs.amazonaws.com/groups/global/AllUsers"
	uriAuthenticatedUsers = "http://acs.amazonaws.com/groups/global/AuthenticatedUsers"
)

func bucketACLTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("bucket_acl_default", bucketACLDefault),
		b.add("bucket_acl_canned", bucketACLCanned),
		b.add("bucket_acl_canned_during_create", bucketACLCannedDuringCreate, "fails_on_aws"),
		b.add("bucket_acl_canned_publicreadwrite", bucketACLCannedPublicReadWrite),
		b.add("bucket_acl_canned_authenticatedread", bucketACLCannedAuthenticatedRead),
		b.add("bucket_acl_canned_private_to_private", bucketACLCannedPrivateToPrivate),
		b.add("bucket_acl_grant_userid_fullcontrol", bucketACLGrantUseridFullControl, "fails_on_aws"),
		b.add("bucket_acl_grant_userid_read", bucketACLGrantUseridRead, "fails_on_aws"),
		b.add("bucket_acl_grant_userid_readacp", bucketACLGrantUseridReadACP, "fails_on_aws"),
		b.add("bucket_acl_grant_userid_write", bucketACLGrantUseridWrite, "fails_on_aws"),
		b.add("bucket_acl_grant_userid_writeacp", bucketACLGrantUseridWriteACP, "fails_on_aws"),
		b.add("bucket_acl_grant_nonexist_user", bucketACLGrantNonexistUser, "fails_on_aws"),
		b.add("bucket_acl_grant_email", bucketACLGrantEmail, "fails_on_aws"),
		b.add("bucket_acl_grant_email_not_exist", bucketACLGrantEmailNotExist),
		b.add("bucket_acl_revoke_all", bucketACLRevokeAll),
	}
}

func getBucketACL(e *fixture.Env, bucket string) *awss3.GetBucketAclOutput {
	out, err := e.Client().GetBucketAcl(e.Ctx(), &awss3.GetBucketAclInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "get bucket acl")
	return out
}

// newBucketCanned creates a bucket with a canned ACL.
func newBucketCanned(e *fixture.Env, acl types.BucketCannedACL) string {
	return setupBucketACL(e, acl)
}

// ownerGrant is the grant every bucket has for its own owner.
func ownerGrant(e *fixture.Env) wantGrant {
	return wantGrant{
		permission:  types.PermissionFullControl,
		id:          e.Cfg.Main.UserID,
		displayName: e.Cfg.Main.DisplayName,
		grantType:   types.TypeCanonicalUser,
	}
}

// groupGrant is a grant to one of the predefined groups, which carries a URI
// rather than a user id.
func groupGrant(permission types.Permission, uri string) wantGrant {
	return wantGrant{permission: permission, grantType: types.TypeGroup, uri: uri}
}

func bucketACLDefault(e *fixture.Env) {
	bucket := e.NewBucket()

	acl := getBucketACL(e, bucket)
	s3util.Equal(e.T, aws.ToString(acl.Owner.DisplayName), e.Cfg.Main.DisplayName, "owner display name")
	s3util.Equal(e.T, aws.ToString(acl.Owner.ID), e.Cfg.Main.UserID, "owner id")
	checkGrants(e, acl.Grants, []wantGrant{ownerGrant(e)})
}

// bucketACLCanned and bucketACLCannedDuringCreate are the same check in
// upstream: both create with public-read and read the ACL back.
func bucketACLCanned(e *fixture.Env) {
	bucket := newBucketCanned(e, types.BucketCannedACLPublicRead)
	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{
		groupGrant(types.PermissionRead, uriAllUsers),
		ownerGrant(e),
	})
}

func bucketACLCannedDuringCreate(e *fixture.Env) { bucketACLCanned(e) }

func bucketACLCannedPublicReadWrite(e *fixture.Env) {
	bucket := newBucketCanned(e, types.BucketCannedACLPublicReadWrite)
	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{
		groupGrant(types.PermissionRead, uriAllUsers),
		groupGrant(types.PermissionWrite, uriAllUsers),
		ownerGrant(e),
	})
}

func bucketACLCannedAuthenticatedRead(e *fixture.Env) {
	bucket := newBucketCanned(e, types.BucketCannedACLAuthenticatedRead)
	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{
		groupGrant(types.PermissionRead, uriAuthenticatedUsers),
		ownerGrant(e),
	})
}

func bucketACLCannedPrivateToPrivate(e *fixture.Env) {
	bucket := newBucketCanned(e, types.BucketCannedACLPrivate)

	// Setting private on an already-private bucket is a no-op, not an error.
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    types.BucketCannedACLPrivate,
	})
	s3util.NoError(e.T, err, "put bucket acl private")
	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{ownerGrant(e)})
}

// bucketACLGrantUserid grants the alt user a permission on a new bucket and
// checks the ACL reads back, mirroring upstream's _bucket_acl_grant_userid.
func bucketACLGrantUserid(e *fixture.Env, permission types.Permission) string {
	bucket := e.NewBucket()

	grants := append(getBucketACL(e, bucket).Grants, types.Grant{
		Grantee:    &types.Grantee{ID: aws.String(e.Cfg.Alt.UserID), Type: types.TypeCanonicalUser},
		Permission: permission,
	})
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner: &types.Owner{
				DisplayName: aws.String(e.Cfg.Main.DisplayName),
				ID:          aws.String(e.Cfg.Main.UserID),
			},
			Grants: grants,
		},
	})
	s3util.NoError(e.T, err, "put bucket acl")

	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{
		{
			permission:  permission,
			id:          e.Cfg.Alt.UserID,
			displayName: e.Cfg.Alt.DisplayName,
			grantType:   types.TypeCanonicalUser,
		},
		ownerGrant(e),
	})
	return bucket
}

// The alt user's capabilities on a granted bucket. Each returns the error, so
// callers assert on allowed or denied.
func altHeadBucket(e *fixture.Env, bucket string) error {
	_, err := e.AltClient().HeadBucket(e.Ctx(), &awss3.HeadBucketInput{Bucket: aws.String(bucket)})
	return err
}

func altGetBucketACL(e *fixture.Env, bucket string) error {
	_, err := e.AltClient().GetBucketAcl(e.Ctx(), &awss3.GetBucketAclInput{Bucket: aws.String(bucket)})
	return err
}

func altPutObject(e *fixture.Env, bucket string) error {
	_, err := e.AltClient().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo-write"),
		Body:   readerOf("bar"),
	})
	return err
}

func altPutBucketACL(e *fixture.Env, bucket string) error {
	_, err := e.AltClient().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    types.BucketCannedACLPublicRead,
	})
	return err
}

// accessDenied asserts an operation was refused, mirroring upstream's
// check_access_denied.
func accessDenied(e *fixture.Env, err error, what string) {
	if err == nil {
		e.T.Errorf("%s was allowed, want AccessDenied", what)
		return
	}
	if status, code := s3util.StatusAndCode(err); status != 403 || code != "AccessDenied" {
		e.T.Errorf("%s = status %d code %s, want 403 AccessDenied", what, status, code)
	}
}

func bucketACLGrantUseridFullControl(e *fixture.Env) {
	bucket := bucketACLGrantUserid(e, types.PermissionFullControl)

	s3util.NoError(e.T, altHeadBucket(e, bucket), "alt head bucket")
	s3util.NoError(e.T, altGetBucketACL(e, bucket), "alt get bucket acl")
	s3util.NoError(e.T, altPutObject(e, bucket), "alt put object")
	s3util.NoError(e.T, altPutBucketACL(e, bucket), "alt put bucket acl")

	// Full control does not transfer ownership.
	acl := getBucketACL(e, bucket)
	s3util.Equal(e.T, aws.ToString(acl.Owner.ID), e.Cfg.Main.UserID, "owner id")
	s3util.Equal(e.T, aws.ToString(acl.Owner.DisplayName), e.Cfg.Main.DisplayName, "owner display name")
}

func bucketACLGrantUseridRead(e *fixture.Env) {
	bucket := bucketACLGrantUserid(e, types.PermissionRead)

	s3util.NoError(e.T, altHeadBucket(e, bucket), "alt head bucket")
	accessDenied(e, altGetBucketACL(e, bucket), "alt get bucket acl")
	accessDenied(e, altPutObject(e, bucket), "alt put object")
	accessDenied(e, altPutBucketACL(e, bucket), "alt put bucket acl")
}

func bucketACLGrantUseridReadACP(e *fixture.Env) {
	bucket := bucketACLGrantUserid(e, types.PermissionReadAcp)

	accessDenied(e, altHeadBucket(e, bucket), "alt head bucket")
	s3util.NoError(e.T, altGetBucketACL(e, bucket), "alt get bucket acl")
	accessDenied(e, altPutObject(e, bucket), "alt put object")
	accessDenied(e, altPutBucketACL(e, bucket), "alt put bucket acl")
}

func bucketACLGrantUseridWrite(e *fixture.Env) {
	bucket := bucketACLGrantUserid(e, types.PermissionWrite)

	accessDenied(e, altHeadBucket(e, bucket), "alt head bucket")
	accessDenied(e, altGetBucketACL(e, bucket), "alt get bucket acl")
	s3util.NoError(e.T, altPutObject(e, bucket), "alt put object")
	accessDenied(e, altPutBucketACL(e, bucket), "alt put bucket acl")
}

func bucketACLGrantUseridWriteACP(e *fixture.Env) {
	bucket := bucketACLGrantUserid(e, types.PermissionWriteAcp)

	accessDenied(e, altHeadBucket(e, bucket), "alt head bucket")
	accessDenied(e, altGetBucketACL(e, bucket), "alt get bucket acl")
	accessDenied(e, altPutObject(e, bucket), "alt put object")
	s3util.NoError(e.T, altPutBucketACL(e, bucket), "alt put bucket acl")
}

// putBucketACLGrantee writes an ACL with one extra grantee and returns the
// error, for the tests about grantees the server should reject.
func putBucketACLGrantee(e *fixture.Env, bucket string, grantee *types.Grantee) error {
	grants := append(getBucketACL(e, bucket).Grants, types.Grant{
		Grantee:    grantee,
		Permission: types.PermissionFullControl,
	})
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner: &types.Owner{
				DisplayName: aws.String(e.Cfg.Main.DisplayName),
				ID:          aws.String(e.Cfg.Main.UserID),
			},
			Grants: grants,
		},
	})
	return err
}

func bucketACLGrantNonexistUser(e *fixture.Env) {
	bucket := e.NewBucket()

	err := putBucketACLGrantee(e, bucket, &types.Grantee{
		ID:   aws.String("Foo"),
		Type: types.TypeCanonicalUser,
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
}

func bucketACLGrantEmail(e *fixture.Env) {
	bucket := e.NewBucket()

	// A grantee named by email resolves to the user's canonical id.
	err := putBucketACLGrantee(e, bucket, &types.Grantee{
		EmailAddress: aws.String(e.Cfg.Alt.Email),
		Type:         types.TypeAmazonCustomerByEmail,
	})
	s3util.NoError(e.T, err, "put bucket acl by email")

	checkGrants(e, getBucketACL(e, bucket).Grants, []wantGrant{
		{
			permission:  types.PermissionFullControl,
			id:          e.Cfg.Alt.UserID,
			displayName: e.Cfg.Alt.DisplayName,
			grantType:   types.TypeCanonicalUser,
		},
		ownerGrant(e),
	})
}

func bucketACLGrantEmailNotExist(e *fixture.Env) {
	bucket := e.NewBucket()

	err := putBucketACLGrantee(e, bucket, &types.Grantee{
		EmailAddress: aws.String("doesnotexist@dreamhost.com.invalid"),
		Type:         types.TypeAmazonCustomerByEmail,
	})
	s3util.ErrorIs(e.T, err, 400, "UnresolvableGrantByEmailAddress")
}

func bucketACLRevokeAll(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	before := getBucketACL(e, bucket)

	// Revoking everything, including the owner's own access, is allowed.
	_, err := e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner:  before.Owner,
			Grants: []types.Grant{},
		},
	})
	s3util.NoError(e.T, err, "put empty bucket acl")
	s3util.Equal(e.T, len(getBucketACL(e, bucket).Grants), 0, "grant count")

	// Put the grants back so the bucket can still be cleaned up.
	_, err = e.Client().PutBucketAcl(e.Ctx(), &awss3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		AccessControlPolicy: &types.AccessControlPolicy{
			Owner:  before.Owner,
			Grants: before.Grants,
		},
	})
	s3util.NoError(e.T, err, "restore bucket acl")
}
