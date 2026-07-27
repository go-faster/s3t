// Command s3t runs the S3 compatibility suite against an S3-compatible server.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-faster/s3t/internal/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := cmd.Root().ExecuteContext(ctx); err != nil {
		// Interrupt is not a failure: report it the way a shell would.
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "s3t:", err)
		os.Exit(1)
	}
}
