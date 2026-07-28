package client

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// WithHeaders sets headers on a single request, replacing upstream's
//
//	lf = (lambda **kwargs: kwargs['params']['headers'].update(headers))
//	client.meta.events.register('before-call.s3.PutObject', lf)
//
// Pass it as a per-call option:
//
//	client.PutObject(ctx, in, client.WithHeaders(map[string]string{...}))
//
// Headers go on before signing, because that is where botocore's before-call
// hook runs: it mutates the request dict, and only then does the endpoint
// build and sign a request from it. The difference is not cosmetic — a value
// the signature covers is a request the server can parse and reject on its
// merits, while one added afterwards is just a signature mismatch. It also
// means a header the signer owns, Authorization or X-Amz-Date, is overwritten
// rather than sent, exactly as upstream.
func WithHeaders(h map[string]string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("s3t:SetHeaders",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					out middleware.FinalizeOutput, md middleware.Metadata, err error,
				) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						for name, value := range h {
							req.Header.Set(name, value)
						}
					}
					return next.HandleFinalize(ctx, in)
				}), middleware.Before)
		})
	}
}

// WithoutHeader removes a header the SDK would otherwise send, replacing
// upstream's remove_header handlers. Like WithHeaders it runs before signing,
// so a header the signer adds for itself comes back.
func WithoutHeader(names ...string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("s3t:RemoveHeaders",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					out middleware.FinalizeOutput, md middleware.Metadata, err error,
				) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						for _, name := range names {
							req.Header.Del(name)
						}
					}
					return next.HandleFinalize(ctx, in)
				}), middleware.Before)
		})
	}
}

// WithQuery adds raw query parameters, replacing upstream's
//
//	kwargs['params']['url'] += "&max-keys=blah"
//
// Added in the Build step, before signing, because SigV4 covers the query
// string: appending afterwards would produce a signature mismatch and the
// server would answer 403 instead of the 400 the test is checking for.
func WithQuery(params map[string]string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Build.Add(middleware.BuildMiddlewareFunc("s3t:SetQuery",
				func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (
					out middleware.BuildOutput, md middleware.Metadata, err error,
				) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						q := req.URL.Query()
						for k, v := range params {
							q.Set(k, v)
						}
						req.URL.RawQuery = q.Encode()
					}
					return next.HandleBuild(ctx, in)
				}), middleware.After)
		})
	}
}

// WithPathReplace rewrites part of the request path, replacing upstream's
//
//	url = kwargs['params']['url'].replace(valid_bucket_name, invalid_name)
//
// It exists for the bucket-naming tests: an SDK that validates a bucket name
// locally never puts it on the wire, and those tests are about what the server
// does with it. Upstream hits the same wall with botocore and solves it the
// same way.
//
// The rewrite is inserted immediately before the signer. It cannot go in the
// Build step: the bucket is put into the path during endpoint resolution, which
// runs later, so at Build time the path does not contain the name yet. It also
// cannot go after signing, since the signature covers the path.
func WithPathReplace(old, replacement string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("s3t:PathReplace",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					out middleware.FinalizeOutput, md middleware.Metadata, err error,
				) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						req.URL.Path = strings.Replace(req.URL.Path, old, replacement, 1)
						req.URL.RawPath = ""
					}
					return next.HandleFinalize(ctx, in)
				}), "Signing", middleware.Before)
		})
	}
}

// WithChunkedTransferEncoding sends the body with chunked transfer encoding.
//
// net/http decides framing from the request rather than from a header, so
// setting Transfer-Encoding by hand does nothing: the length has to be cleared
// so the transport chunks instead.
//
// That has to happen before signing. Content-Length is among the headers SigV4
// covers, so clearing it afterwards leaves a signature over a request that was
// never sent, and the server answers SignatureDoesNotMatch. Upstream hooks
// before-sign for the same reason.
func WithChunkedTransferEncoding() func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("s3t:Chunked",
				func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
					out middleware.FinalizeOutput, md middleware.Metadata, err error,
				) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						req.ContentLength = -1
						req.Header.Del("Content-Length")
					}
					return next.HandleFinalize(ctx, in)
				}), "Signing", middleware.Before)
		})
	}
}
