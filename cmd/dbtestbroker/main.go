package main

// dbtestbroker is a subprocess used when testing with dbtestutil. It handles cloning Postgres databases for tests and
// is a separate process so that if the main test process dies (panics, times out, or is killed), we still clean up
// the test databases and don't leave them in Postgres.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtestutil/broker"
)

func main() {
	connectCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server, err := broker.NewService(connectCtx, os.Stdin, os.Stdout)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "\nDBTESTBROKER: %s\n", err.Error())
		// nolint: gocritic
		os.Exit(1)
	}
	signalCtx, signalCancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer signalCancel()
	err = server.Serve(signalCtx)
	if err != nil && !xerrors.Is(err, context.Canceled) && !xerrors.Is(err, io.EOF) {
		_, _ = fmt.Fprintf(os.Stderr, "\nDBTESTBROKER: %s\n", err.Error())
	}
}
