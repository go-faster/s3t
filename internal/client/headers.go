package client

import (
	"context"

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
// Headers are applied after signing, so a value the server must reject can be
// sent without the signature covering it. Use WithSignedHeaders where the test
// expects the header to be signed.
func WithHeaders(h map[string]string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, setHeaders(h, finalizeAfterSigning))
	}
}

// WithSignedHeaders sets headers before signing, so the signature covers them.
func WithSignedHeaders(h map[string]string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, setHeaders(h, middleware.Before))
	}
}

// WithoutHeader removes a header the SDK would otherwise send, replacing
// upstream's remove_header handlers. Removal happens after signing, since the
// point is usually to make the request malformed.
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
				}), finalizeAfterSigning)
		})
	}
}

// finalizeAfterSigning places a middleware at the end of the Finalize step,
// which runs after the signer.
const finalizeAfterSigning = middleware.After

func setHeaders(h map[string]string, pos middleware.RelativePosition) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
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
			}), pos)
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
