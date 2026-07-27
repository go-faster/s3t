// Package client builds S3 clients configured to behave like the boto3
// clients the upstream suite uses.
package client

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/go-faster/s3t/internal/config"
)

// signingRegion is the region used to sign requests.
//
// Upstream sets no region at all and lets botocore fall back to us-east-1;
// the "api_name" config option is a zonegroup name used in bucket-location
// assertions, not a signing region, so it deliberately is not used here.
const signingRegion = "us-east-1"

// Factory builds clients for the users defined in the config.
type Factory struct {
	cfg  *config.Config
	http *http.Client
}

// New returns a Factory sharing one HTTP transport across every client it
// builds.
func New(cfg *config.Config) *Factory {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// The pool defaults to 2 idle connections per host, which would
	// serialize concurrent tests onto two sockets.
	transport.MaxIdleConnsPerHost = 64
	transport.TLSClientConfig = &tls.Config{
		//nolint:gosec // ssl_verify = False is a supported config for test servers
		InsecureSkipVerify: !cfg.SSLVerify,
		MinVersion:         tls.VersionTLS12,
	}
	return &Factory{
		cfg:  cfg,
		http: &http.Client{Transport: transport, Timeout: time.Minute},
	}
}

// Main returns a client for the "s3 main" user.
func (f *Factory) Main() *s3.Client { return f.forUser(f.cfg.Main) }

// Alt returns a client for the "s3 alt" user, used by tests that check one
// user cannot reach another's resources.
func (f *Factory) Alt() *s3.Client { return f.forUser(f.cfg.Alt) }

// Tenant returns a client for the "s3 tenant" user.
func (f *Factory) Tenant() *s3.Client { return f.forUser(f.cfg.Tenant) }

func (f *Factory) forUser(u config.User) *s3.Client {
	return s3.NewFromConfig(aws.Config{
		Region:      signingRegion,
		Credentials: credentials.NewStaticCredentialsProvider(u.AccessKey, u.SecretKey, ""),
		HTTPClient:  f.http,

		// Retries are off. The suite asserts on 5xx and on throttling
		// responses; retrying would turn those assertions into hangs or
		// silent passes.
		Retryer: func() aws.Retryer { return aws.NopRetryer{} },

		// The modern SDK otherwise adds a CRC32 checksum header and
		// aws-chunked framing to every upload, changing the bytes on the
		// wire that the header, auth and checksum tests inspect. boto3
		// sends neither by default.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenSupported,
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(f.cfg.Endpoint)
		// boto3 addresses a custom endpoint_url path-style.
		o.UsePathStyle = true
		o.APIOptions = append(o.APIOptions, captureStatus)
	})
}
