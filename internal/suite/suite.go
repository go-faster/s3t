// Package suite collects every ported test package.
//
// It is the single place to touch when a test package is added.
package suite

import (
	"github.com/go-faster/s3t/internal/client"
	"github.com/go-faster/s3t/internal/config"
	"github.com/go-faster/s3t/internal/harness"
	"github.com/go-faster/s3t/tests/headers"
	"github.com/go-faster/s3t/tests/s3"
)

// Registry builds the registry of every ported test.
func Registry(cfg *config.Config, clients *client.Factory) (*harness.Registry, error) {
	var tests []harness.Test
	tests = append(tests, s3.Tests(cfg, clients)...)
	tests = append(tests, headers.Tests(cfg, clients)...)
	return harness.NewRegistry(tests)
}
