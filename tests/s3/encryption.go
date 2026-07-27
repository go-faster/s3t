package s3

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// markerEncryption is on every test in this file, matching upstream.
const markerEncryption = "encryption"

// SSE-C key material, verbatim from upstream so the same bytes go on the wire.
const (
	sseCAlgorithm = "AES256"
	sseCKey       = "pO3upElrwuEXSoFwCfnZPdSsmt/xWeFa0N9KgDijwVs="
	sseCKeyMD5    = "DWygnHRtgiJ77HCm+1rvHw=="
)

// sseCHeaders are the customer-key headers as upstream sends them: raw, through
// a before-call hook, rather than as SDK parameters. Keeping them raw means the
// same bytes reach the server, without an SDK's own base64 or MD5 handling in
// between.
//
// They are applied before signing, as boto3's before-call hook does. That is
// not cosmetic: these maps also carry Content-Type, which the SDK sets and
// signs itself, so setting it afterwards invalidates the signature.
func sseCHeaders() map[string]string {
	return map[string]string{
		"x-amz-server-side-encryption-customer-algorithm": sseCAlgorithm,
		"x-amz-server-side-encryption-customer-key":       sseCKey,
		"x-amz-server-side-encryption-customer-key-md5":   sseCKeyMD5,
	}
}

func sseCCopySourceHeaders() map[string]string {
	return map[string]string{
		"x-amz-copy-source-server-side-encryption-customer-algorithm": sseCAlgorithm,
		"x-amz-copy-source-server-side-encryption-customer-key":       sseCKey,
		"x-amz-copy-source-server-side-encryption-customer-key-md5":   sseCKeyMD5,
	}
}

func sseKMSHeaders(keyID string) map[string]string {
	return map[string]string{
		"x-amz-server-side-encryption":                "aws:kms",
		"x-amz-server-side-encryption-aws-kms-key-id": keyID,
	}
}

// encMode is one of the source encryption modes upstream parametrizes over.
type encMode struct {
	name string
	// put are the headers used when writing the source object.
	put func(e *fixture.Env) map[string]string
	// copySource are the headers naming the source key on a copy.
	copySource func(e *fixture.Env) map[string]string
}

// encSourceModes are the three source modes the gate covers. Upstream also
// defines sse-s3, which is not in the allow-list.
var encSourceModes = []encMode{
	{name: "unencrypted"},
	{
		name:       "sse-c",
		put:        func(*fixture.Env) map[string]string { return sseCHeaders() },
		copySource: func(*fixture.Env) map[string]string { return sseCCopySourceHeaders() },
	},
	{
		name: "sse-kms",
		put:  func(e *fixture.Env) map[string]string { return sseKMSHeaders(e.Cfg.KMSKeyID) },
	},
}

// encSizes are the object sizes upstream parametrizes copy_enc over.
var encSizes = []int{1, 1024, 1024 * 1024, 8 * 1024 * 1024}

func encryptionTests(b builder) []harness.Test {
	var out []harness.Test

	// Names carry the pytest parameter suffix so the node IDs match the
	// allow-list exactly: only the unencrypted destination and the STANDARD
	// storage class are in the gate.
	for _, mode := range encSourceModes {
		for _, size := range encSizes {
			name := fmt.Sprintf("copy_enc[%s-unencrypted-STANDARD-STANDARD-%d]", mode.name, size)
			out = append(out, b.add(name, copyEnc(mode, size),
				markerEncryption, "fails_on_dbstore", "fails_on_aws"))
		}
		name := fmt.Sprintf("copy_part_enc[%s-unencrypted-STANDARD-STANDARD-8388608]", mode.name)
		out = append(out, b.add(name, copyPartEnc(mode, 8*1024*1024),
			markerEncryption, "fails_on_dbstore", "fails_on_aws"))
	}

	out = append(out,
		b.add("encrypted_transfer_1b", encryptedTransfer(1), markerEncryption, "fails_on_dbstore"),
		b.add("encrypted_transfer_1kb", encryptedTransfer(1024), markerEncryption, "fails_on_dbstore"),
		b.add("encrypted_transfer_13b", encryptedTransfer(13), markerEncryption, "fails_on_dbstore"),
		b.add("encrypted_transfer_1MB", encryptedTransfer(1024*1024), markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_transfer_1b", sseKMSTransfer(1), markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_transfer_1kb", sseKMSTransfer(1024), markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_transfer_13b", sseKMSTransfer(13), markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_transfer_1MB", sseKMSTransfer(1024*1024), markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_present", sseKMSPresent, markerEncryption, "fails_on_dbstore"),
		b.add("encryption_sse_c_multipart_upload",
			multipartEncUpload(30*1024*1024, defaultPartSize, "text/plain",
				func(*fixture.Env) map[string]string { return sseCHeaders() }, true),
			markerEncryption, "fails_on_dbstore"),
		b.add("encryption_sse_c_unaligned_multipart_upload",
			multipartEncUpload(30*1024*1024, 5*1024*1024+500*1024, "text/plain",
				func(*fixture.Env) map[string]string { return sseCHeaders() }, true),
			markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_multipart_upload",
			multipartEncUpload(30*1024*1024, defaultPartSize, "text/plain",
				func(e *fixture.Env) map[string]string { return sseKMSHeaders(e.Cfg.KMSKeyID) }, false),
			markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_multipart_invalid_chunks_1", sseKMSMultipartInvalidChunks("text/bla"),
			markerEncryption, "fails_on_dbstore"),
		b.add("sse_kms_multipart_invalid_chunks_2", sseKMSMultipartInvalidChunks("text/plain"),
			markerEncryption, "fails_on_dbstore"),
	)
	return out
}

// encKey is the key every encryption test writes, as upstream does.
const encKey = "testobj"

// putEncrypted writes an object with the given encryption headers.
func putEncrypted(e *fixture.Env, bucket, body string, headers map[string]string) {
	opts := []func(*awss3.Options){}
	if len(headers) > 0 {
		opts = append(opts, client.WithSignedHeaders(headers))
	}
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(encKey),
		Body:         strings.NewReader(body),
		StorageClass: types.StorageClassStandard,
	}, opts...)
	s3util.NoError(e.T, err, "put object")
}

// getEncrypted reads an object back with the given encryption headers.
func getEncrypted(e *fixture.Env, bucket, key string, headers map[string]string) string {
	opts := []func(*awss3.Options){}
	if len(headers) > 0 {
		opts = append(opts, client.WithSignedHeaders(headers))
	}
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, opts...)
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()
	return readAll(e, out.Body)
}

func copyEnc(mode encMode, size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		data := strings.Repeat("A", size)

		var putHeaders map[string]string
		if mode.put != nil {
			putHeaders = mode.put(e)
		}
		putEncrypted(e, bucket, data, putHeaders)

		// The destination is unencrypted, so the copy carries only the
		// headers naming the source key.
		destBucket := e.NewBucket()
		opts := []func(*awss3.Options){}
		if mode.copySource != nil {
			opts = append(opts, client.WithSignedHeaders(mode.copySource(e)))
		}
		_, err := e.Client().CopyObject(e.Ctx(), &awss3.CopyObjectInput{
			Bucket:       aws.String(destBucket),
			Key:          aws.String("testobj2"),
			CopySource:   copySource(bucket, "testobj"),
			StorageClass: types.StorageClassStandard,
		}, opts...)
		s3util.NoError(e.T, err, "copy object")

		s3util.Equal(e.T, getEncrypted(e, destBucket, "testobj2", nil) == data, true,
			"copy holds the source data")
	}
}

func copyPartEnc(mode encMode, size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		data := strings.Repeat("A", size)

		var putHeaders map[string]string
		if mode.put != nil {
			putHeaders = mode.put(e)
		}
		putEncrypted(e, bucket, data, putHeaders)

		destBucket := e.NewBucket()
		created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
			Bucket:       aws.String(destBucket),
			Key:          aws.String("testobj2"),
			StorageClass: types.StorageClassStandard,
		})
		s3util.NoError(e.T, err, "create multipart upload")
		s3util.EqualNow(e.T, aws.ToString(created.UploadId) != "", true, "upload id is set")

		opts := []func(*awss3.Options){}
		if mode.copySource != nil {
			opts = append(opts, client.WithSignedHeaders(mode.copySource(e)))
		}
		part, err := e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
			Bucket:     aws.String(destBucket),
			Key:        aws.String("testobj2"),
			PartNumber: aws.Int32(1),
			UploadId:   created.UploadId,
			CopySource: copySource(bucket, "testobj"),
		}, opts...)
		s3util.NoError(e.T, err, "upload part copy")

		completeMultipart(e, destBucket, "testobj2", aws.ToString(created.UploadId),
			[]types.CompletedPart{{ETag: part.CopyPartResult.ETag, PartNumber: aws.Int32(1)}})

		s3util.Equal(e.T, getEncrypted(e, destBucket, "testobj2", nil) == data, true,
			"copy holds the source data")
	}
}

// encryptedTransfer writes and reads back with SSE-C headers, mirroring
// upstream's _test_encryption_sse_customer_write.
func encryptedTransfer(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		data := strings.Repeat("A", size)

		putEncrypted(e, bucket, data, sseCHeaders())
		s3util.Equal(e.T, getEncrypted(e, bucket, "testobj", sseCHeaders()) == data, true, "body")
	}
}

// sseKMSTransfer writes and reads back with SSE-KMS headers, mirroring
// upstream's _test_sse_kms_customer_write.
func sseKMSTransfer(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		data := strings.Repeat("A", size)

		putEncrypted(e, bucket, data, sseKMSHeaders(e.Cfg.KMSKeyID))
		s3util.Equal(e.T, getEncrypted(e, bucket, "testobj", nil) == data, true, "body")
	}
}

func sseKMSPresent(e *fixture.Env) {
	bucket := e.NewBucket()
	data := strings.Repeat("A", 100)

	putEncrypted(e, bucket, data, sseKMSHeaders(e.Cfg.KMSKeyID))
	s3util.Equal(e.T, getEncrypted(e, bucket, "testobj", nil), data, "body")
}

// multipartEnc runs a multipart upload with encryption headers on the create,
// part and complete calls, mirroring upstream's _multipart_upload_enc.
func multipartEnc(e *fixture.Env, bucket, key string, size, partSize int,
	initHeaders, partHeaders map[string]string, metadata map[string]string,
) (uploadID, data string, parts []types.CompletedPart) {
	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		Metadata: metadata,
	}, client.WithSignedHeaders(initHeaders))
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID = aws.ToString(created.UploadId)

	var content strings.Builder
	for i, offset := 0, 0; offset < size; i, offset = i+1, offset+partSize {
		part := strings.Repeat("A", min(partSize, size-offset))
		content.WriteString(part)

		partNum := int32(i + 1)
		out, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(partNum),
			Body:       strings.NewReader(part),
		}, client.WithSignedHeaders(partHeaders))
		s3util.NoError(e.T, err, fmt.Sprintf("upload part %d", partNum))
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(partNum)})
	}
	return uploadID, content.String(), parts
}

// encMultipartKey is the key the multipart encryption tests write.
const encMultipartKey = "multipart_enc"

// multipartEncUpload is the shared body of the SSE-C and SSE-KMS multipart
// tests: upload with encryption headers, complete, then read back and verify.
func multipartEncUpload(size, partSize int, contentType string,
	headers func(e *fixture.Env) map[string]string, readHeaders bool,
) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		metadata := map[string]string{"foo": "bar"}

		enc := headers(e)
		enc["Content-Type"] = contentType

		uploadID, data, parts := multipartEnc(e, bucket, encMultipartKey, size, partSize,
			enc, enc, metadata)

		_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
			Bucket:          aws.String(bucket),
			Key:             aws.String(encMultipartKey),
			UploadId:        aws.String(uploadID),
			MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
		}, client.WithSignedHeaders(enc))
		s3util.NoError(e.T, err, "complete multipart upload")

		listed := listV2(e, &awss3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
			Prefix: aws.String(encMultipartKey),
		})
		s3util.EqualNow(e.T, len(listed.Contents), 1, "object count")
		s3util.Equal(e.T, aws.ToInt64(listed.Contents[0].Size), int64(size), "object size")

		// SSE-C needs the key to read back; SSE-KMS does not.
		var getHeaders map[string]string
		if readHeaders {
			getHeaders = headers(e)
		}
		opts := []func(*awss3.Options){}
		if len(getHeaders) > 0 {
			opts = append(opts, client.WithSignedHeaders(getHeaders))
		}
		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(encMultipartKey),
		}, opts...)
		s3util.NoError(e.T, err, "get object")
		s3util.EqualMetadata(e.T, out.Metadata, metadata, "metadata")
		s3util.Equal(e.T, aws.ToString(out.ContentType), contentType, "content type")

		body := readAll(e, out.Body)
		_ = out.Body.Close()
		s3util.Equal(e.T, body == data, true, "body matches the uploaded content")
		s3util.Equal(e.T, int64(len(body)), aws.ToInt64(out.ContentLength), "body length")
	}
}

// sseKMSMultipartInvalidChunks uploads with one KMS key on the create and a
// different one on the parts. Upstream asserts only that the upload proceeds:
// the mismatch is accepted rather than rejected.
func sseKMSMultipartInvalidChunks(contentType string) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()

		initHeaders := sseKMSHeaders(e.Cfg.KMSKeyID)
		initHeaders["Content-Type"] = contentType
		partHeaders := sseKMSHeaders(e.Cfg.SecondaryKMSKeyID)

		multipartEnc(e, bucket, encMultipartKey, 30*1024*1024, defaultPartSize,
			initHeaders, partHeaders, map[string]string{"foo": "bar"})
	}
}
