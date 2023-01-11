package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
)

func main() {
	rand.Seed(time.Now().UnixMicro())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add a custom SIGQUIT handler that outputs to stderr and a well-known file
	// in the home directory. This also prevents SIGQUITs from killing the CLI.
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGQUIT)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigs:
			}

			buf := make([]byte, 10_000_000)
			stacklen := runtime.Stack(buf, true)

			_, _ = fmt.Fprintf(os.Stderr, "SIGQUIT:\n%s\n", buf[:stacklen])

			// Write to a well-known file.
			dir, err := os.UserHomeDir()
			if err != nil {
				dir = os.TempDir()
			}
			fpath := filepath.Join(dir, fmt.Sprintf("coder-agent-%s.dump", time.Now().Format("2006-01-02T15:04:05.000Z")))
			_, _ = fmt.Fprintf(os.Stderr, "writing dump to %q\n", fpath)

			f, err := os.Create(fpath)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "failed to open dump file: %v\n", err.Error())
				continue
			}
			_, err = f.Write(buf[:stacklen])
			_ = f.Close()
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "failed to open dump file: %v\n", err.Error())
				continue
			}
		}
	}()

	cmd, err := cli.Root(cli.AGPL()).ExecuteC()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		cobraErr := cli.FormatCobraError(err, cmd)
		_, _ = fmt.Fprintln(os.Stderr, cobraErr)
		os.Exit(1)
	}
}
