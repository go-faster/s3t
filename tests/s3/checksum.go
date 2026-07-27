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

// markerChecksum is upstream's marker for the client-visible checksum family
// (x-amz-checksum-*), which is a second checksum with its own algorithms and
// negotiation — not the content MD5 behind ETag.
const markerChecksum = "checksum"

// checksumKey is the object these tests write.
const checksumKey = "myobj"

func checksumTests(b builder) []harness.Test {
	return []harness.Test{
		b.add("object_checksum_sha256", objectChecksumSHA256, markerChecksum),
		b.add("object_checksum_crc64nvme", objectChecksumCRC64NVME, markerChecksum),
	}
}

// objectChecksum is the shape both tests share: upstream writes 1 KiB of a
// single repeated byte with a precomputed digest, so the expected values are
// constants rather than something the test computes — a test that computed the
// digest the same way the server does would pass against a server that got the
// algorithm wrong.
func objectChecksum(
	algorithm types.ChecksumAlgorithm,
	want string,
	set func(*awss3.PutObjectInput, *string),
	get func(*awss3.HeadObjectOutput) *string,
) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		body := strings.Repeat("A", 1024)

		in := &awss3.PutObjectInput{
			Bucket:            aws.String(bucket),
			Key:               aws.String(checksumKey),
			Body:              strings.NewReader(body),
			ChecksumAlgorithm: algorithm,
		}
		set(in, aws.String(want))

		put, err := e.Client().PutObject(e.Ctx(), in)
		s3util.NoError(e.T, err, "put object")

		head, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(checksumKey),
		})
		s3util.NoError(e.T, err, "head object")
		s3util.Equal(e.T, get(head) == nil, true,
			"HEAD must omit the checksum unless the request enables it")

		enabled, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket:       aws.String(bucket),
			Key:          aws.String(checksumKey),
			ChecksumMode: types.ChecksumModeEnabled,
		})
		s3util.NoError(e.T, err, "head object with checksum mode")
		s3util.Equal(e.T, aws.ToString(get(enabled)), want, "checksum on HEAD")

		// The digest the client sent is what the server must verify against;
		// a wrong one has to be refused rather than stored and echoed back.
		bad := &awss3.PutObjectInput{
			Bucket:            aws.String(bucket),
			Key:               aws.String(checksumKey),
			Body:              strings.NewReader(body),
			ChecksumAlgorithm: algorithm,
		}
		set(bad, aws.String("bad"))

		_, err = e.Client().PutObject(e.Ctx(), bad)
		s3util.ErrorIs(e.T, err, 400, "BadDigest")

		_ = put
	}
}

// objectChecksumSHA256 ports upstream's test_object_checksum_sha256.
var objectChecksumSHA256 = objectChecksum(
	types.ChecksumAlgorithmSha256,
	"arcu6553sHVAiX4MjW0j7I7vD4w6R+Gz9Ok0Q9lTa+0=",
	func(in *awss3.PutObjectInput, v *string) { in.ChecksumSHA256 = v },
	func(out *awss3.HeadObjectOutput) *string { return out.ChecksumSHA256 },
)

// objectChecksumCRC64NVME ports upstream's test_object_checksum_crc64nvme.
var objectChecksumCRC64NVME = objectChecksum(
	types.ChecksumAlgorithmCrc64nvme,
	"Qeh8oXvGiSo=",
	func(in *awss3.PutObjectInput, v *string) { in.ChecksumCRC64NVME = v },
	func(out *awss3.HeadObjectOutput) *string { return out.ChecksumCRC64NVME },
)
