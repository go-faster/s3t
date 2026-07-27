package client

import (
	"context"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-faster/errors"
)

// StatusAndCode extracts the HTTP status and S3 error code from a failed
// request, mirroring upstream's _get_status_and_error_code.
//
// A nil error yields (0, ""), so a test that expected a failure and got none
// compares against something obviously wrong rather than a plausible zero
// value.
func StatusAndCode(err error) (status int, code string) {
	if err == nil {
		return 0, ""
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) {
		status = respErr.HTTPStatusCode()
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code = apiErr.ErrorCode()
	}
	return status, code
}

// statusKey identifies the captured status code in operation metadata.
type statusKey struct{}

// captureStatus records the HTTP status of every response in the operation's
// result metadata.
//
// The SDK exposes the status on errors but not on successes, while the suite
// asserts on both (upstream reads ResponseMetadata.HTTPStatusCode, which boto3
// populates either way). Without this, tests checking for 200 versus 204 have
// nothing to read.
func captureStatus(stack *middleware.Stack) error {
	return stack.Deserialize.Add(middleware.DeserializeMiddlewareFunc("s3t:CaptureStatus",
		func(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler) (
			out middleware.DeserializeOutput, md middleware.Metadata, err error,
		) {
			out, md, err = next.HandleDeserialize(ctx, in)
			if resp, ok := out.RawResponse.(*smithyhttp.Response); ok {
				md.Set(statusKey{}, resp.StatusCode)
			}
			return out, md, err
		}), middleware.Before)
}

// Status returns the HTTP status recorded for a successful operation, from its
// ResultMetadata.
func Status(md middleware.Metadata) int {
	status, _ := md.Get(statusKey{}).(int)
	return status
}
