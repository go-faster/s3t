package s3

import (
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/fixture"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/internal/s3util"
)

func atomicTests(b builder) []harness.Test {
	// Every test here races a second operation against an in-flight one and
	// assumes nothing else is touching the server, so they run in the
	// serial phase.
	const serial = harness.MarkerSerial
	return []harness.Test{
		b.add("atomic_read_1mb", atomicRead(1024*1024), serial),
		b.add("atomic_read_4mb", atomicRead(4*1024*1024), serial),
		b.add("atomic_read_8mb", atomicRead(8*1024*1024), serial),
		b.add("atomic_write_1mb", atomicWrite(1024*1024), serial),
		b.add("atomic_write_4mb", atomicWrite(4*1024*1024), serial),
		b.add("atomic_write_8mb", atomicWrite(8*1024*1024), serial),
		b.add("atomic_dual_write_1mb", atomicDualWrite(1024*1024), serial),
		b.add("atomic_dual_write_4mb", atomicDualWrite(4*1024*1024), serial),
		b.add("atomic_dual_write_8mb", atomicDualWrite(8*1024*1024), serial),
		b.add("atomic_conditional_write_1mb", atomicConditionalWrite(1024*1024), serial, "fails_on_aws"),
		b.add("atomic_dual_conditional_write_1mb", atomicDualConditionalWrite(1024*1024), serial, "fails_on_rgw"),
		b.add("atomic_write_bucket_gone", atomicWriteBucketGone, serial, "fails_on_rgw"),
		b.add("atomic_multipart_upload_write", atomicMultipartUploadWrite, serial),
	}
}

// interruptReader yields size copies of char and runs interrupt once, when the
// last byte is read. It is upstream's FakeWriteFile, seek included.
//
// Seeking matters more than it looks. SigV4 hashes the body before sending,
// reading it to the end and seeking back, which means the hook fires while the
// payload is being *hashed* -- before the request is sent at all. That is what
// boto3 does too, since FakeWriteFile is seekable, so these tests observe a
// concurrent operation that completed before the write started rather than one
// racing it.
//
// Streaming the body unsigned instead does race the write, and produces
// different results: go-faster/fs answers 500 rather than 404 when its bucket
// is deleted mid-upload. Upstream never exercises that, and phase 2 is about
// matching the gate, so this keeps boto3's behavior. The stronger property is
// worth testing one day, but not under a name that means something else.
type interruptReader struct {
	char      byte
	size      int
	offset    int
	interrupt func()
	fired     bool
}

// Seek rewinds after hashing. The interrupt does not fire twice: FakeWriteFile
// only triggers on reaching the end, and fired guards the rest.
func (r *interruptReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.offset = int(offset)
	case io.SeekCurrent:
		r.offset += int(offset)
	case io.SeekEnd:
		r.offset = r.size + int(offset)
	}
	return int64(r.offset), nil
}

func (r *interruptReader) Read(p []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	n := min(len(p), r.size-r.offset)
	for i := range n {
		p[i] = r.char
	}
	r.offset += n

	if r.interrupt != nil && !r.fired && r.offset == r.size && n > 0 {
		r.fired = true
		r.interrupt()
	}
	return n, nil
}

// putInterruptible writes an interruptReader as the body.
func putInterruptible(e *fixture.Env, bucket, key string, r *interruptReader, opts ...func(*awss3.Options)) error {
	_, err := e.Client().PutObject(e.Ctx(), &awss3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(int64(r.size)),
	}, opts...)
	return err
}

// atomicKey is the key every atomic test writes, as upstream does.
const atomicKey = "testobj"

// verifyAtomicKeyData checks an object is exactly size copies of char,
// mirroring upstream's _verify_atomic_key_data.
func verifyAtomicKeyData(e *fixture.Env, bucket string, size int, char byte) {
	out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(atomicKey),
	})
	s3util.NoError(e.T, err, "get object")
	defer func() { _ = out.Body.Close() }()

	body := readAll(e, out.Body)
	s3util.Equal(e.T, len(body), size, "object size")
	s3util.Equal(e.T, body == strings.Repeat(string(char), size), true,
		"object holds only "+string(char))
}

// requireFired fails the test if the hook never ran: without it these tests
// would pass while racing nothing at all.
func requireFired(e *fixture.Env, r *interruptReader) {
	s3util.Equal(e.T, r.fired, true, "interrupt fired during the write")
}

func atomicRead(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		const key = atomicKey

		s3util.NoError(e.T, putInterruptible(e, bucket, key,
			&interruptReader{char: 'A', size: size}), "put A")

		// While reading the A's back, overwrite the object with B's.
		out, err := e.Client().GetObject(e.Ctx(), &awss3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		s3util.NoError(e.T, err, "get object")

		overwritten := false
		buf := make([]byte, 64*1024)
		for {
			n, err := out.Body.Read(buf)
			if n > 0 && !overwritten {
				overwritten = true
				s3util.NoError(e.T, putInterruptible(e, bucket, key,
					&interruptReader{char: 'B', size: size}), "put B mid-read")
			}
			if err != nil {
				break
			}
		}
		_ = out.Body.Close()
		s3util.Equal(e.T, overwritten, true, "overwrite happened mid-read")

		verifyAtomicKeyData(e, bucket, size, 'B')
	}
}

func atomicWrite(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		const key = atomicKey

		s3util.NoError(e.T, putInterruptible(e, bucket, key,
			&interruptReader{char: 'A', size: size}), "put A")
		verifyAtomicKeyData(e, bucket, size, 'A')

		// While the B's are still being written, the object must still
		// read as all A's: a partial write must not be visible.
		r := &interruptReader{char: 'B', size: size, interrupt: func() {
			verifyAtomicKeyData(e, bucket, size, 'A')
		}}
		s3util.NoError(e.T, putInterruptible(e, bucket, key, r), "put B")
		requireFired(e, r)

		verifyAtomicKeyData(e, bucket, size, 'B')
	}
}

func atomicDualWrite(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		const key = atomicKey
		putObject(e, bucket, key, "")

		// Write B's, and just before they finish, write A's from another
		// request. The result must be one or the other, not a mixture.
		r := &interruptReader{char: 'B', size: size, interrupt: func() {
			s3util.NoError(e.T, putInterruptible(e, bucket, key,
				&interruptReader{char: 'A', size: size}), "put A mid-write")
		}}
		s3util.NoError(e.T, putInterruptible(e, bucket, key, r), "put B")
		requireFired(e, r)

		verifyAtomicKeyData(e, bucket, size, 'B')
	}
}

func atomicConditionalWrite(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		const key = atomicKey

		s3util.NoError(e.T, putInterruptible(e, bucket, key,
			&interruptReader{char: 'A', size: size}), "put A")

		r := &interruptReader{char: 'B', size: size, interrupt: func() {
			verifyAtomicKeyData(e, bucket, size, 'A')
		}}
		s3util.NoError(e.T, putInterruptible(e, bucket, key, r,
			client.WithHeaders(map[string]string{"If-Match": "*"})), "put B if-match *")
		requireFired(e, r)

		verifyAtomicKeyData(e, bucket, size, 'B')
	}
}

func atomicDualConditionalWrite(size int) func(*fixture.Env) {
	return func(e *fixture.Env) {
		bucket := e.NewBucket()
		const key = atomicKey

		first := &interruptReader{char: 'A', size: size}
		s3util.NoError(e.T, putInterruptible(e, bucket, key, first), "put A")
		verifyAtomicKeyData(e, bucket, size, 'A')

		head, err := e.Client().HeadObject(e.Ctx(), &awss3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		s3util.NoError(e.T, err, "head object")
		etagA := strings.Trim(aws.ToString(head.ETag), `"`)

		// C's are written conditional on the object still being A. Part
		// way through, B's replace it, so the condition no longer holds
		// and the C write must be refused.
		r := &interruptReader{char: 'C', size: size, interrupt: func() {
			s3util.NoError(e.T, putInterruptible(e, bucket, key,
				&interruptReader{char: 'B', size: size}), "put B mid-write")
		}}
		err = putInterruptible(e, bucket, key, r,
			client.WithHeaders(map[string]string{"If-Match": etagA}))
		requireFired(e, r)
		s3util.ErrorIs(e.T, err, 412, "PreconditionFailed")

		verifyAtomicKeyData(e, bucket, size, 'B')
	}
}

func atomicWriteBucketGone(e *fixture.Env) {
	bucket := e.NewBucket()

	// The bucket is deleted while the object is still being uploaded.
	r := &interruptReader{char: 'A', size: 1024 * 1024, interrupt: func() {
		_, err := e.Client().DeleteBucket(e.Ctx(), &awss3.DeleteBucketInput{
			Bucket: aws.String(bucket),
		})
		s3util.NoError(e.T, err, "delete bucket mid-write")
	}}
	err := putInterruptible(e, bucket, "foo", r)
	requireFired(e, r)
	s3util.ErrorIs(e.T, err, 404, "NoSuchBucket")
}

func atomicMultipartUploadWrite(e *fixture.Env) {
	bucket := e.NewBucket()
	putObject(e, bucket, "foo", "bar")

	created, err := e.Client().CreateMultipartUpload(e.Ctx(), &awss3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("foo"),
	})
	s3util.NoError(e.T, err, "create multipart upload")

	// An open multipart upload must not disturb the existing object.
	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body during upload")

	_, err = e.Client().AbortMultipartUpload(e.Ctx(), &awss3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String("foo"),
		UploadId: created.UploadId,
	})
	s3util.NoError(e.T, err, "abort multipart upload")

	s3util.Equal(e.T, getObjectBody(e, bucket, "foo"), "bar", "body after abort")
}
