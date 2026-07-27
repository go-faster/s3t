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

// exitInterrupted is the conventional shell status for death by SIGINT.
const exitInterrupted = 130

func main() {
	ctx, stop := interruptContext()
	defer stop()

	if err := cmd.Root().ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(exitInterrupted)
		}
		fmt.Fprintln(os.Stderr, "s3t:", err)
		os.Exit(1)
	}
}

// interruptContext cancels on the first interrupt and exits on the second.
//
// The first signal has to unwind rather than exit: tests are mid-flight and
// their buckets still need deleting. But cleanup can take a while against a
// slow server, and someone who hits Ctrl-C twice wants out now -- without the
// second stage they would have to reach for SIGKILL, which leaves every bucket
// behind, exactly what the first stage was protecting.
func interruptContext() (ctx context.Context, stop func()) {
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-ch
		fmt.Fprintln(os.Stderr, "\ns3t: interrupted, cleaning up (interrupt again to exit now)")
		cancel()

		<-ch
		fmt.Fprintln(os.Stderr, "s3t: interrupted again, exiting; buckets may be left behind")
		os.Exit(exitInterrupted)
	}()

	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}
