package s3

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// defaultPartSize is upstream's default, and the S3 minimum for every part but
// the last.
const defaultPartSize = 5 * 1024 * 1024

func multipartTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("multipart_upload_empty", multipartUploadEmpty),
		b.add("multipart_upload_contents", multipartUploadContents, "fails_on_dbstore"),
		b.add("multipart_upload_multiple_sizes", multipartUploadMultipleSizes),
		b.add("multipart_upload_size_too_small", multipartUploadSizeTooSmall),
		b.add("multipart_upload_overwrite_existing_object", multipartUploadOverwriteExistingObject),
		b.add("multipart_upload_incorrect_etag", multipartUploadIncorrectEtag),
		b.add("multipart_upload_missing_part", multipartUploadMissingPart),
		b.add("multipart_upload_resend_part", multipartUploadResendPart, "fails_on_dbstore"),
		b.add("multipart_upload_complete_without_create", multipartUploadCompleteWithoutCreate, "fails_on_dbstore"),
		b.add("list_multipart_upload", listMultipartUpload, "fails_on_dbstore"),
		b.add("multipart_copy_small", multipartCopySmall, markerCopy, "fails_on_dbstore"),
		b.add("multipart_copy_invalid_range", multipartCopyInvalidRange, markerCopy),
		b.add("multipart_copy_improper_range", multipartCopyImproperRange, "fails_on_rgw"),
		b.add("multipart_copy_without_range", multipartCopyWithoutRange, markerCopy),
		b.add("multipart_copy_special_names", multipartCopySpecialNames, markerCopy, "fails_on_dbstore"),
		b.add("multipart_copy_multiple_sizes", multipartCopyMultipleSizes, markerCopy, "fails_on_dbstore"),
		b.add("upload_part_copy_percent_encoded_key", uploadPartCopyPercentEncodedKey),
	}
}

// multipartOpts carries the optional arguments upstream's _multipart_upload
// takes as keywords.
type multipartOpts struct {
	partSize    int
	contentType string
	metadata    map[string]string
	// resendParts are zero-based part indexes to upload a second time,
	// exercising a client retrying a part.
	resendParts []int
}

// multipartUpload creates an upload and sends its parts, returning the upload
// id, the full content and the completed part list. It mirrors upstream's
// _multipart_upload.
func multipartUpload(e *fixture.Env, bucket, key string, size int, opts multipartOpts) (
	uploadID, data string, parts []types.CompletedPart,
) {
	partSize := opts.partSize
	if partSize == 0 {
		partSize = defaultPartSize
	}

	in := &awss3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if opts.contentType != "" || opts.metadata != nil {
		in.ContentType = aws.String(opts.contentType)
		in.Metadata = opts.metadata
	}
	created, err := e.Client().CreateMultipartUpload(e.Ctx(), in)
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID = aws.ToString(created.UploadId)

	var content strings.Builder
	for i, offset := 0, 0; offset < size; i, offset = i+1, offset+partSize {
		part := s3util.RandomString(min(partSize, size-offset))
		content.WriteString(part)

		partNum := int32(i + 1)
		out, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(partNum),
			Body:       strings.NewReader(part),
		})
		s3util.NoError(e.T, err, fmt.Sprintf("upload part %d", partNum))
		parts = append(parts, types.CompletedPart{
			ETag:       out.ETag,
			PartNumber: aws.Int32(partNum),
		})

		for _, resend := range opts.resendParts {
			if resend != i {
				continue
			}
			_, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
				Bucket:     aws.String(bucket),
				Key:        aws.String(key),
				UploadId:   aws.String(uploadID),
				PartNumber: aws.Int32(partNum),
				Body:       strings.NewReader(part),
			})
			s3util.NoError(e.T, err, fmt.Sprintf("resend part %d", partNum))
		}
	}
	return uploadID, content.String(), parts
}

// completeMultipart finishes an upload and fails the test if it errors.
func completeMultipart(e *fixture.Env, bucket, key, uploadID string, parts []types.CompletedPart) {
	_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	s3util.NoError(e.T, err, "complete multipart upload")
}

func multipartUploadEmpty(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	uploadID, _, _ := multipartUpload(e, bucket, key, 0, multipartOpts{})

	_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	s3util.ErrorIs(e.T, err, 400, "MalformedXML")
}

func multipartUploadContents(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"
	const numParts = 3

	payload := strings.Repeat(s3util.RandomString(5), 1024*1024)

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID := aws.ToString(created.UploadId)

	var parts []types.CompletedPart
	for i := range numParts {
		out, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(int32(i + 1)),
			Body:       strings.NewReader(payload),
		})
		s3util.NoError(e.T, err, "upload part")
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(int32(i + 1))})
	}

	lastPayload := strings.Repeat("123", 1024*1024)
	out, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(numParts + 1),
		Body:       strings.NewReader(lastPayload),
	})
	s3util.NoError(e.T, err, "upload last part")
	parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(numParts + 1)})

	completed, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	s3util.NoError(e.T, err, "complete multipart upload")
	s3util.Equal(e.T, aws.ToString(completed.ETag) != "", true, "etag is set")

	want := strings.Repeat(payload, numParts) + lastPayload
	s3util.Equal(e.T, getObjectBody(e, bucket, key) == want, true, "body matches the parts")
}

func multipartUploadMultipleSizes(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	const mb = 1024 * 1024
	for _, size := range []int{
		5 * mb,
		5*mb + 100*1024,
		5*mb + 600*1024,
		10*mb + 100*1024,
		10*mb + 600*1024,
		10 * mb,
	} {
		uploadID, _, parts := multipartUpload(e, bucket, key, size, multipartOpts{})
		completeMultipart(e, bucket, key, uploadID, parts)
	}
}

func multipartUploadSizeTooSmall(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	// Parts below the 5 MiB minimum are only rejected at completion.
	uploadID, _, parts := multipartUpload(e, bucket, key, 100*1024, multipartOpts{partSize: 10 * 1024})

	_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	s3util.ErrorIs(e.T, err, 400, "EntityTooSmall")
}

func multipartUploadOverwriteExistingObject(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"
	const numParts = 2
	payload := strings.Repeat("12345", 1024*1024)

	putObject(e, bucket, key, payload)

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID := aws.ToString(created.UploadId)

	var parts []types.CompletedPart
	for i := range numParts {
		out, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(int32(i + 1)),
			Body:       strings.NewReader(payload),
		})
		s3util.NoError(e.T, err, "upload part")
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(int32(i + 1))})
	}
	completeMultipart(e, bucket, key, uploadID, parts)

	want := strings.Repeat(payload, numParts)
	s3util.Equal(e.T, getObjectBody(e, bucket, key) == want, true, "body is the multipart content")
}

func multipartUploadIncorrectEtag(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID := aws.ToString(created.UploadId)

	_, err = e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(1),
		Body:       strings.NewReader("\x00"),
	})
	s3util.NoError(e.T, err, "upload part")

	_, err = e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: aws.String("ffffffffffffffffffffffffffffffff"), PartNumber: aws.Int32(1)},
		}},
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidPart")
}

func multipartUploadMissingPart(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID := aws.ToString(created.UploadId)

	part, err := e.Client().UploadPart(e.Ctx(), &awss3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(1),
		Body:       strings.NewReader("\x00"),
	})
	s3util.NoError(e.T, err, "upload part")

	// The part exists but is claimed under a number that was never uploaded.
	_, err = e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: part.ETag, PartNumber: aws.Int32(9999)},
		}},
	})
	s3util.ErrorIs(e.T, err, 400, "InvalidPart")
}

func multipartUploadResendPart(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"
	const objLen = 30 * 1024 * 1024

	for _, resend := range [][]int{{0}, {1}, {2}, {1, 2}, {0, 1, 2, 3, 4, 5}} {
		checkUploadMultipartResend(e, bucket, key, objLen, resend)
	}
}

// checkUploadMultipartResend uploads with some parts sent twice and checks the
// result is unaffected, mirroring upstream's _check_upload_multipart_resend.
func checkUploadMultipartResend(e *fixture.Env, bucket, key string, objLen int, resendParts []int) {
	const contentType = "text/bla"
	metadata := map[string]string{"foo": "bar"}

	uploadID, data, parts := multipartUpload(e, bucket, key, objLen, multipartOpts{
		contentType: contentType,
		metadata:    metadata,
		resendParts: resendParts,
	})
	completeMultipart(e, bucket, key, uploadID, parts)

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, aws.ToString(out.ContentType), contentType, "content type")
	s3util.EqualMetadata(e.T, out.Metadata, metadata, "metadata")
	body := readAll(e, out.Body)
	_ = out.Body.Close()

	s3util.Equal(e.T, int64(len(body)), aws.ToInt64(out.ContentLength), "body length")
	s3util.Equal(e.T, body == data, true, "body matches the uploaded content")

	checkContentUsingRange(e, bucket, key, data, 1000000)
	checkContentUsingRange(e, bucket, key, data, 10000000)
}

// checkContentUsingRange reads an object back in steps and compares each range
// against the expected content, mirroring upstream's _check_content_using_range.
func checkContentUsingRange(e *fixture.Env, bucket, key, data string, step int) {
	size := len(data)
	for ofs := 0; ofs < size; ofs += step {
		toRead := min(step, size-ofs)
		end := ofs + toRead - 1

		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Range:  aws.String(fmt.Sprintf("bytes=%d-%d", ofs, end)),
		})
		s3util.NoError(e.T, err, "get object range")
		s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(toRead), "range length")
		body := readAll(e, out.Body)
		_ = out.Body.Close()
		s3util.Equal(e.T, body == data[ofs:end+1], true, "range content")
	}
}

func multipartUploadCompleteWithoutCreate(e *fixture.Env) {
	bucket := e.NewBucket()

	_, err := e.Client().CompleteMultipartUpload(e.Ctx(), &awss3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String("mymultipart"),
		UploadId: aws.String("abc1234def"),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: aws.String("1234"), PartNumber: aws.Int32(1)},
		}},
	})
	s3util.ErrorIs(e.T, err, 404, "NoSuchUpload")
}

func listMultipartUpload(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "mymultipart"
	const key2 = "mymultipart2"
	const mb = 1024 * 1024

	id1, _, _ := multipartUpload(e, bucket, key, 5*mb, multipartOpts{})
	id2, _, _ := multipartUpload(e, bucket, key, 6*mb, multipartOpts{})
	id3, _, _ := multipartUpload(e, bucket, key2, 5*mb, multipartOpts{})

	out, err := e.Client().ListMultipartUploads(e.Ctx(), &awss3.ListMultipartUploadsInput{
		Bucket: aws.String(bucket),
	})
	s3util.NoError(e.T, err, "list multipart uploads")

	got := map[string]bool{}
	for _, u := range out.Uploads {
		got[aws.ToString(u.UploadId)] = true
	}
	for _, id := range []string{id1, id2, id3} {
		s3util.Equal(e.T, got[id], true, "upload "+id+" is listed")
	}

	for _, u := range []struct{ key, id string }{{key, id1}, {key, id2}, {key2, id3}} {
		_, err := e.Client().AbortMultipartUpload(e.Ctx(), &awss3.AbortMultipartUploadInput{
			Bucket:   aws.String(bucket),
			Key:      aws.String(u.key),
			UploadId: aws.String(u.id),
		})
		s3util.NoError(e.T, err, "abort multipart upload")
	}
}

// createKeyWithRandomContent writes an object of random bytes, mirroring
// upstream's _create_key_with_random_content.
func createKeyWithRandomContent(e *fixture.Env, key string, size int, bucket string) string {
	if bucket == "" {
		bucket = e.NewBucket()
	}
	putObject(e, bucket, key, s3util.RandomString(size))
	return bucket
}

// multipartCopy builds a multipart upload whose parts are ranges of another
// object, mirroring upstream's _multipart_copy.
func multipartCopy(e *fixture.Env, srcBucket, srcKey, destBucket, destKey string, size, partSize int) (
	uploadID string, parts []types.CompletedPart,
) {
	if partSize == 0 {
		partSize = defaultPartSize
	}

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "create multipart upload")
	uploadID = aws.ToString(created.UploadId)

	for i, start := 0, 0; start < size; i, start = i+1, start+partSize {
		end := min(start+partSize-1, size-1)
		partNum := int32(i + 1)

		out, err := e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
			Bucket:          aws.String(destBucket),
			Key:             aws.String(destKey),
			CopySource:      copySource(srcBucket, srcKey),
			CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
			PartNumber:      aws.Int32(partNum),
			UploadId:        aws.String(uploadID),
		})
		s3util.NoError(e.T, err, fmt.Sprintf("upload part copy %d", partNum))
		parts = append(parts, types.CompletedPart{
			ETag:       out.CopyPartResult.ETag,
			PartNumber: aws.Int32(partNum),
		})
	}
	return uploadID, parts
}

// checkKeyContent compares a copied object against the head of its source,
// mirroring upstream's _check_key_content.
func checkKeyContent(e *fixture.Env, srcKey, srcBucket, destKey, destBucket string) {
	src, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(srcBucket),
		Key:    aws.String(srcKey),
	})
	s3util.NoError(e.T, err, "get source object")
	srcSize := aws.ToInt64(src.ContentLength)
	_ = src.Body.Close()

	dest, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "get dest object")
	destSize := aws.ToInt64(dest.ContentLength)
	destData := readAll(e, dest.Body)
	_ = dest.Body.Close()

	s3util.Equal(e.T, srcSize >= destSize, true, "source is at least as large as the copy")

	ranged, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(srcBucket),
		Key:    aws.String(srcKey),
		Range:  aws.String(fmt.Sprintf("bytes=0-%d", destSize-1)),
	})
	s3util.NoError(e.T, err, "get source range")
	srcData := readAll(e, ranged.Body)
	_ = ranged.Body.Close()

	s3util.Equal(e.T, srcData == destData, true, "copy matches the source range")
}

func multipartCopySmall(e *fixture.Env) {
	const srcKey = "foo"
	srcBucket := createKeyWithRandomContent(e, srcKey, 7*1024*1024, "")
	destBucket := e.NewBucket()
	const destKey = "mymultipart"
	const size = 1

	uploadID, parts := multipartCopy(e, srcBucket, srcKey, destBucket, destKey, size, 0)
	completeMultipart(e, destBucket, destKey, uploadID, parts)

	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
	_ = out.Body.Close()

	checkKeyContent(e, srcKey, srcBucket, destKey, destBucket)
}

func multipartCopyInvalidRange(e *fixture.Env) {
	const srcKey = "source"
	bucket := createKeyWithRandomContent(e, srcKey, 5, "")

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("dest"),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	_, err = e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String("dest"),
		UploadId:        created.UploadId,
		CopySource:      copySource(bucket, srcKey),
		CopySourceRange: aws.String("bytes=0-21"),
		PartNumber:      aws.Int32(1),
	})
	// Upstream accepts either status here, since servers disagree.
	s3util.ErrorIsOneOf(e.T, err, []int{400, 416}, "InvalidRange")
}

func multipartCopyImproperRange(e *fixture.Env) {
	const srcKey = "source"
	bucket := createKeyWithRandomContent(e, srcKey, 5, "")

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("dest"),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	for _, rng := range []string{
		"0-2",
		"bytes=0",
		"bytes=hello-world",
		"bytes=0-bar",
		"bytes=hello-",
		"bytes=0-2,3-5",
	} {
		_, err := e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
			Bucket:          aws.String(bucket),
			Key:             aws.String("dest"),
			UploadId:        created.UploadId,
			CopySource:      copySource(bucket, srcKey),
			CopySourceRange: aws.String(rng),
			PartNumber:      aws.Int32(1),
		})
		s3util.ErrorIs(e.T, err, 400, "InvalidArgument")
	}
}

func multipartCopyWithoutRange(e *fixture.Env) {
	const srcKey = "source"
	srcBucket := createKeyWithRandomContent(e, srcKey, 10, "")
	destBucket := e.NewBucket()
	const destKey = "mymultipartcopy"

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	// No CopySourceRange: the whole source becomes one part.
	out, err := e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
		Bucket:     aws.String(destBucket),
		Key:        aws.String(destKey),
		CopySource: copySource(srcBucket, srcKey),
		PartNumber: aws.Int32(1),
		UploadId:   created.UploadId,
	})
	s3util.NoError(e.T, err, "upload part copy")

	completeMultipart(e, destBucket, destKey, aws.ToString(created.UploadId), []types.CompletedPart{
		{ETag: out.CopyPartResult.ETag, PartNumber: aws.Int32(1)},
	})

	got, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(destBucket),
		Key:    aws.String(destKey),
	})
	s3util.NoError(e.T, err, "get object")
	s3util.Equal(e.T, aws.ToInt64(got.ContentLength), 10, "content length")
	_ = got.Body.Close()

	checkKeyContent(e, srcKey, srcBucket, destKey, destBucket)
}

func multipartCopySpecialNames(e *fixture.Env) {
	srcBucket := e.NewBucket()
	destBucket := e.NewBucket()
	const destKey = "mymultipart"
	const size = 1

	for _, srcKey := range []string{" ", "_", "__", "?versionId"} {
		createKeyWithRandomContent(e, srcKey, 7*1024*1024, srcBucket)

		uploadID, parts := multipartCopy(e, srcBucket, srcKey, destBucket, destKey, size, 0)
		completeMultipart(e, destBucket, destKey, uploadID, parts)

		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(destBucket),
			Key:    aws.String(destKey),
		})
		s3util.NoError(e.T, err, "get object")
		s3util.Equal(e.T, aws.ToInt64(out.ContentLength), int64(size), "content length")
		_ = out.Body.Close()

		checkKeyContent(e, srcKey, srcBucket, destKey, destBucket)
	}
}

func multipartCopyMultipleSizes(e *fixture.Env) {
	const srcKey = "foo"
	srcBucket := createKeyWithRandomContent(e, srcKey, 12*1024*1024, "")
	destBucket := e.NewBucket()
	const destKey = "mymultipart"

	const mb = 1024 * 1024
	for _, size := range []int{
		5 * mb,
		5*mb + 100*1024,
		5*mb + 600*1024,
		10*mb + 100*1024,
		10*mb + 600*1024,
		10 * mb,
	} {
		uploadID, parts := multipartCopy(e, srcBucket, srcKey, destBucket, destKey, size, 0)
		completeMultipart(e, destBucket, destKey, uploadID, parts)
		checkKeyContent(e, srcKey, srcBucket, destKey, destBucket)
	}
}

func uploadPartCopyPercentEncodedKey(e *fixture.Env) {
	bucket := e.NewBucket()
	const key = "anyfile.txt"
	const encodedKey = "anyfilename%25.txt"
	const rawKey = "anyfilename%.txt"

	for _, k := range []string{encodedKey, key} {
		_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(k),
			Body:        strings.NewReader("foo"),
			ContentType: aws.String("text/plain"),
		})
		s3util.NoError(e.T, err, "put object "+k)
	}

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	// Copying from the raw, unencoded name must fail: the source name is
	// the encoded one.
	_, err = e.Client().UploadPartCopy(e.Ctx(), &awss3.UploadPartCopyInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		PartNumber: aws.Int32(1),
		UploadId:   created.UploadId,
		CopySource: copySource(bucket, rawKey),
	})
	s3util.Equal(e.T, err != nil, true, "upload part copy from the raw key fails")

	// The target must be untouched by the failed copy.
	s3util.Equal(e.T, getObjectBody(e, bucket, key), "foo", "body")
}
