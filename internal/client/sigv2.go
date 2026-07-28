package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // the S3 v2 signature is defined over SHA-1
	"encoding/base64"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-faster/errors"

	"github.com/go-faster/s3t/internal/config"
)

// V2 returns a client for the "s3 main" user signing with the S3 v2 scheme,
// replacing upstream's
//
//	get_v2_client()  # Config(signature_version='s3')
//
// The scheme is what test_headers.py's auth_aws2 tests are about: unlike SigV4
// it signs the Date header, so a skewed or malformed date reaches the server
// under a valid signature and gets an answer about the date rather than about
// the signature.
// The checksum calculation is off because the SDK carries the checksum in a
// trailer, framing the body as aws-chunked and announcing it with an
// x-amz-content-sha256 of STREAMING-UNSIGNED-PAYLOAD-TRAILER. That machinery
// belongs to SigV4; a v2-signed request that carries it is rejected as
// MalformedTrailerError. botocore's v2 client sends the checksum as an
// ordinary header instead, and never a trailer, so no trailer is the closer
// match to what upstream puts on the wire.
func (f *Factory) V2() *s3.Client {
	return f.forUser(f.cfg.Main, signV2(f.cfg.Main), WithoutRequestChecksum())
}

// signV2 replaces the SDK's SigV4 signer, mirroring botocore's HmacV1Auth.
func signV2(u config.User) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			_, err := stack.Finalize.Swap("Signing", middleware.FinalizeMiddlewareFunc("s3t:SignV2",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					out middleware.FinalizeOutput, md middleware.Metadata, err error,
				) {
					req, ok := in.Request.(*smithyhttp.Request)
					if !ok {
						return out, md, errors.Errorf("sign v2: unexpected request type %T", in.Request)
					}
					// A v2 request carries no payload hash; the SDK
					// adds one for SigV4 further up the stack.
					req.Header.Del("X-Amz-Content-Sha256")
					// HmacV1Auth replaces whatever Date the caller
					// set, which is why the date tests use
					// x-amz-date instead.
					req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
					req.Header.Set("Authorization", "AWS "+u.AccessKey+":"+signatureV2(u.SecretKey, req))
					return next.HandleFinalize(ctx, in)
				}))
			return err
		})
	}
}

func signatureV2(secret string, req *smithyhttp.Request) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(stringToSignV2(req)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// stringToSignV2 builds botocore's canonical_string: the method, the three
// standard headers, the x-amz-* headers, and the resource.
func stringToSignV2(req *smithyhttp.Request) string {
	var b strings.Builder
	b.WriteString(strings.ToUpper(req.Method))
	b.WriteByte('\n')
	for _, name := range []string{"Content-Md5", "Content-Type", "Date"} {
		b.WriteString(strings.TrimSpace(req.Header.Get(name)))
		b.WriteByte('\n')
	}
	if custom := canonicalAmzHeaders(req.Header); custom != "" {
		b.WriteString(custom)
		b.WriteByte('\n')
	}
	// Upstream also appends the sub-resource query arguments -- ?acl,
	// ?uploads and the rest of its QSAOfInterest list. Nothing signs a
	// request with one through this client: the v2 tests write objects and
	// create buckets, and the presigned v2 tests use a different signer
	// again. A sub-resource request would be answered 403, not signed
	// wrongly and accepted.
	b.WriteString(req.URL.EscapedPath())
	return b.String()
}

// canonicalAmzHeaders renders the x-amz-* headers, lowercased and sorted by
// name, one per line.
func canonicalAmzHeaders(h http.Header) string {
	var names []string
	for name := range h {
		if strings.HasPrefix(strings.ToLower(name), "x-amz-") {
			names = append(names, strings.ToLower(name))
		}
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		values := h.Values(name)
		trimmed := make([]string, len(values))
		for i, v := range values {
			trimmed[i] = strings.TrimSpace(v)
		}
		lines = append(lines, name+":"+strings.Join(trimmed, ","))
	}
	return strings.Join(lines, "\n")
}
