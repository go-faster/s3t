package s3

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

// partSize is upstream's: every part but the last must be at least 5 MiB.
const partSize = 5 * 1024 * 1024

// cksumCase is one algorithm's worth of upstream's precomputed digests over
// three parts of 5 MiB of 'A', 'B' and 'C'.
//
// The digests are constants for the same reason the object-level ones are: a
// test that computed them the way the server does would agree with a server
// that computed them wrongly.
type cksumCase struct {
	name      string
	algorithm types.ChecksumAlgorithm
	kind      types.ChecksumType
	part      [3]string
	composite string
}

// The composite cases carry a "-N" suffix, which is the tell that the value is
// a checksum *of the part checksums* rather than of the object: AWS computes
// it over the concatenated part digests and appends the part count. The
// full-object cases carry no suffix, because there the value really is the
// digest of the whole body.
var cksumCases = []cksumCase{
	{
		name: "sha256", algorithm: types.ChecksumAlgorithmSha256, kind: types.ChecksumTypeComposite,
		part: [3]string{
			"275VF5loJr1YYawit0XSHREhkFXYkkPKGuoK0x9VKxI=",
			"mrHwOfjTL5Zwfj74F05HOQGLdUb7E5szdCbxgUSq6NM=",
			"Vw7oB/nKQ5xWb3hNgbyfkvDiivl+U+/Dft48nfJfDow=",
		},
		composite: "uWBwpe1dxI4Vw8Gf0X9ynOdw/SS6VBzfWm9giiv1sf4=-3",
	},
	{
		name: "sha1", algorithm: types.ChecksumAlgorithmSha1, kind: types.ChecksumTypeComposite,
		part: [3]string{
			"iIaTCGbm+vdVjNqIMF2S0T7ibMk=",
			"LS/TJ32bAVKEwRu+sE3X7awh/lk=",
			"6DDwovUaHwrKNXDMzOGbuvj9kxI=",
		},
		composite: "sizjvY4eud3MrcHdZM3cQ/ol39o=-3",
	},
	{
		name: "crc32", algorithm: types.ChecksumAlgorithmCrc32, kind: types.ChecksumTypeFullObject,
		part:      [3]string{"JRTCyQ==", "QoZTGg==", "YAgjqw=="},
		composite: "WgDhBQ==",
	},
	{
		name: "crc32c", algorithm: types.ChecksumAlgorithmCrc32c, kind: types.ChecksumTypeFullObject,
		part:      [3]string{"MDaLrw==", "TH4EZg==", "Z7mBIQ=="},
		composite: "xU+Krw==",
	},
	{
		name: "crc64nvme", algorithm: types.ChecksumAlgorithmCrc64nvme, kind: types.ChecksumTypeFullObject,
		part:      [3]string{"L/E4WYn8v98=", "xW1l19VobYM=", "cK5MnNaWrW4="},
		composite: "i+6LR0y3eFo=",
	},
}

func multipartChecksumTests(b builder) []harness.Test {
	out := make([]harness.Test, 0, len(cksumCases))
	for _, c := range cksumCases {
		out = append(out, b.add("multipart_use_cksum_helper_"+c.name,
			multipartCksumHelper(c), markerChecksum, "fails_on_dbstore"))
	}

	return out
}

// partChecksum reads one algorithm's field off whichever response carries it.
// The SDK models each algorithm as its own field rather than a map, so the
// selection has to be written out once per shape.
type checksums struct {
	crc32, crc32c, crc64, sha1, sha256 *string
}

func (c checksums) pick(a types.ChecksumAlgorithm) string {
	switch a {
	case types.ChecksumAlgorithmCrc32:
		return aws.ToString(c.crc32)
	case types.ChecksumAlgorithmCrc32c:
		return aws.ToString(c.crc32c)
	case types.ChecksumAlgorithmCrc64nvme:
		return aws.ToString(c.crc64)
	case types.ChecksumAlgorithmSha1:
		return aws.ToString(c.sha1)
	case types.ChecksumAlgorithmSha256:
		return aws.ToString(c.sha256)
	default:
		return ""
	}
}

// multipartCksumHelper ports upstream's multipart_checksum_3parts_helper: three
// parts uploaded with their own digests, completed with the composite, then
// read back four ways.
func multipartCksumHelper(c cksumCase) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		key := "mymultipart3"

		create, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
			Bucket:            aws.String(bucket),
			Key:               aws.String(key),
			ChecksumAlgorithm: c.algorithm,
			ChecksumType:      c.kind,
		})
		s3util.NoError(e.T, err, "create multipart upload")
		s3util.Equal(e.T, create.ChecksumAlgorithm, c.algorithm, "checksum algorithm on create")

		uploadID := aws.ToString(create.UploadId)
		completed := make([]types.CompletedPart, 0, 3)

		for i := range 3 {
			body := strings.Repeat(string(rune('A'+i)), partSize)

			in := &awss3.UploadPartInput{
				Bucket:            aws.String(bucket),
				Key:               aws.String(key),
				UploadId:          aws.String(uploadID),
				PartNumber:        aws.Int32(int32(i + 1)), //nolint:gosec // 1..3.
				Body:              strings.NewReader(body),
				ChecksumAlgorithm: c.algorithm,
			}
			applyPartChecksum(in, c.algorithm, c.part[i])

			part, err := e.Client().UploadPart(e.Ctx(), in)
			s3util.NoError(e.T, err, "upload part")

			got := checksums{part.ChecksumCRC32, part.ChecksumCRC32C, part.ChecksumCRC64NVME, part.ChecksumSHA1, part.ChecksumSHA256}
			s3util.Equal(e.T, got.pick(c.algorithm), c.part[i], "part checksum echoed on upload")

			done := types.CompletedPart{
				ETag:       part.ETag,
				PartNumber: aws.Int32(int32(i + 1)), //nolint:gosec // 1..3.
			}
			applyCompletedChecksum(&done, c.algorithm, c.part[i])
			completed = append(completed, done)
		}

		complete := &awss3.CompleteMultipartUploadInput{
			Bucket:          aws.String(bucket),
			Key:             aws.String(key),
			UploadId:        aws.String(uploadID),
			MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
		}
		applyCompleteChecksum(complete, c.algorithm, c.composite)

		done, err := e.Client().CompleteMultipartUpload(e.Ctx(), complete)
		s3util.NoError(e.T, err, "complete multipart upload")
		s3util.Equal(e.T, done.ChecksumType, c.kind, "checksum type on completion")

		gotDone := checksums{done.ChecksumCRC32, done.ChecksumCRC32C, done.ChecksumCRC64NVME, done.ChecksumSHA1, done.ChecksumSHA256}
		s3util.Equal(e.T, gotDone.pick(c.algorithm), c.composite, "composite checksum on completion")

		// HEAD omits the checksum unless the request enables it.
		head, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key),
		})
		s3util.NoError(e.T, err, "head object")

		bare := checksums{head.ChecksumCRC32, head.ChecksumCRC32C, head.ChecksumCRC64NVME, head.ChecksumSHA1, head.ChecksumSHA256}
		s3util.Equal(e.T, bare.pick(c.algorithm), "", "HEAD must omit the checksum by default")

		enabled, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), ChecksumMode: types.ChecksumModeEnabled,
		})
		s3util.NoError(e.T, err, "head object with checksum mode")
		s3util.Equal(e.T, enabled.ChecksumType, c.kind, "checksum type on HEAD")

		gotHead := checksums{enabled.ChecksumCRC32, enabled.ChecksumCRC32C, enabled.ChecksumCRC64NVME, enabled.ChecksumSHA1, enabled.ChecksumSHA256}
		s3util.Equal(e.T, gotHead.pick(c.algorithm), c.composite, "composite checksum on HEAD")

		attrs, err := e.Client().GetObjectAttributes(e.Ctx(), &awss3.GetObjectAttributesInput{
			Bucket:           aws.String(bucket),
			Key:              aws.String(key),
			ObjectAttributes: []types.ObjectAttributes{types.ObjectAttributesChecksum},
		})
		s3util.NoError(e.T, err, "get object attributes")

		if attrs.Checksum == nil {
			e.T.Errorf("object attributes carry no checksum")
			return
		}

		s3util.Equal(e.T, attrs.Checksum.ChecksumType, c.kind, "checksum type in attributes")

		gotAttrs := checksums{
			attrs.Checksum.ChecksumCRC32, attrs.Checksum.ChecksumCRC32C,
			attrs.Checksum.ChecksumCRC64NVME, attrs.Checksum.ChecksumSHA1, attrs.Checksum.ChecksumSHA256,
		}
		s3util.Equal(e.T, gotAttrs.pick(c.algorithm), c.composite, "composite checksum in attributes")

		// Reading one part reports that part's own digest, not the composite.
		for i := range 3 {
			get, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
				Bucket:     aws.String(bucket),
				Key:        aws.String(key),
				PartNumber: aws.Int32(int32(i + 1)), //nolint:gosec // 1..3.
			})
			s3util.NoError(e.T, err, "get object part")

			gotPart := checksums{get.ChecksumCRC32, get.ChecksumCRC32C, get.ChecksumCRC64NVME, get.ChecksumSHA1, get.ChecksumSHA256}
			s3util.Equal(e.T, get.ChecksumType, c.kind, "checksum type on part read")
			s3util.Equal(e.T, gotPart.pick(c.algorithm), c.part[i], "part checksum on part read")

			_ = get.Body.Close()
		}
	}
}

// applyPartChecksum sets the one UploadPart field the algorithm names.
func applyPartChecksum(in *awss3.UploadPartInput, a types.ChecksumAlgorithm, v string) {
	switch a {
	case types.ChecksumAlgorithmCrc32:
		in.ChecksumCRC32 = aws.String(v)
	case types.ChecksumAlgorithmCrc32c:
		in.ChecksumCRC32C = aws.String(v)
	case types.ChecksumAlgorithmCrc64nvme:
		in.ChecksumCRC64NVME = aws.String(v)
	case types.ChecksumAlgorithmSha1:
		in.ChecksumSHA1 = aws.String(v)
	case types.ChecksumAlgorithmSha256:
		in.ChecksumSHA256 = aws.String(v)
	}
}

// applyCompletedChecksum sets the part digest carried in the completion list.
func applyCompletedChecksum(p *types.CompletedPart, a types.ChecksumAlgorithm, v string) {
	switch a {
	case types.ChecksumAlgorithmCrc32:
		p.ChecksumCRC32 = aws.String(v)
	case types.ChecksumAlgorithmCrc32c:
		p.ChecksumCRC32C = aws.String(v)
	case types.ChecksumAlgorithmCrc64nvme:
		p.ChecksumCRC64NVME = aws.String(v)
	case types.ChecksumAlgorithmSha1:
		p.ChecksumSHA1 = aws.String(v)
	case types.ChecksumAlgorithmSha256:
		p.ChecksumSHA256 = aws.String(v)
	}
}

// applyCompleteChecksum sets the composite the completion claims.
func applyCompleteChecksum(in *awss3.CompleteMultipartUploadInput, a types.ChecksumAlgorithm, v string) {
	switch a {
	case types.ChecksumAlgorithmCrc32:
		in.ChecksumCRC32 = aws.String(v)
	case types.ChecksumAlgorithmCrc32c:
		in.ChecksumCRC32C = aws.String(v)
	case types.ChecksumAlgorithmCrc64nvme:
		in.ChecksumCRC64NVME = aws.String(v)
	case types.ChecksumAlgorithmSha1:
		in.ChecksumSHA1 = aws.String(v)
	case types.ChecksumAlgorithmSha256:
		in.ChecksumSHA256 = aws.String(v)
	}
}
