package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// The vector is botocore's own output for the same request, taken by calling
// HmacV1Auth.canonical_string and get_signature with the date pinned:
//
//	auth = HmacV1Auth(Credentials(key, secret))
//	auth._get_date = lambda: 'Tue, 07 Jul 2020 21:53:04 GMT'
//	auth.canonical_string('PUT', urlsplit(url), headers)
//
// Signing is the one place where being close is the same as being wrong, and
// the header values here are the parts that are easy to get close: a repeated
// x-amz- header is trimmed, the standard headers are ordered md5, type, date
// rather than alphabetically, and the resource is the escaped path.
func testV2Request(t *testing.T) *smithyhttp.Request {
	t.Helper()

	u, err := url.Parse("http://localhost:8077/mybucket/foo")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req := &smithyhttp.Request{Request: &http.Request{
		Method: "PUT",
		URL:    u,
		Header: http.Header{},
	}}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-MD5", "rL0Y20xC+Fzt72VPzMSk2A==")
	req.Header.Set("Date", "Tue, 07 Jul 2020 21:53:04 GMT")
	req.Header.Set("x-amz-meta-b", "2")
	req.Header.Set("x-amz-meta-a", " 1 ")
	return req
}

func TestStringToSignV2(t *testing.T) {
	const want = "PUT\n" +
		"rL0Y20xC+Fzt72VPzMSk2A==\n" +
		"text/plain\n" +
		"Tue, 07 Jul 2020 21:53:04 GMT\n" +
		"x-amz-meta-a:1\n" +
		"x-amz-meta-b:2\n" +
		"/mybucket/foo"

	if got := stringToSignV2(testV2Request(t)); got != want {
		t.Errorf("string to sign =\n%q\nwant\n%q", got, want)
	}
}

func TestSignatureV2(t *testing.T) {
	const (
		secret = "h7GhxuBLTrlhVUyxSPUKUV8r/2EI4ngqJxD7iBdBYLhwluN30JaT3Q=="
		want   = "uw8Ub5Oz1j2oe0AP1zoY9vTzQbY="
	)

	if got := signatureV2(secret, testV2Request(t)); got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}
}

// The v2 client has to replace the SigV4 signer rather than run alongside it,
// and it sends no payload hash: a leftover x-amz-content-sha256 would be
// signed as a custom header and change the signature.
func TestV2ClientSigns(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	f := New(cfg)
	if _, err := f.V2().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String("b"),
		Key:    aws.String("k"),
		Body:   strings.NewReader("body"),
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	if want := "AWS " + cfg.Main.AccessKey + ":"; !strings.HasPrefix(got.Get("Authorization"), want) {
		t.Errorf("Authorization = %q, want prefix %q", got.Get("Authorization"), want)
	}
	if got.Get("Date") == "" {
		t.Error("Date header is missing")
	}
	if v := got.Get("X-Amz-Content-Sha256"); v != "" {
		t.Errorf("X-Amz-Content-Sha256 = %q, want it removed", v)
	}
}
