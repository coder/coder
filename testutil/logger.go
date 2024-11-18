package testutil

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
)

// Logger returns a "standard" testing logger, with debug level and common flaky
// errors ignored.
func Logger(t testing.TB) slog.Logger {
	return slogtest.Make(
		t, &slogtest.Options{IgnoreErrorFn: IgnoreLoggedError},
	).Leveled(slog.LevelDebug)
}

func IgnoreLoggedError(entry slog.SinkEntry) bool {
	err, ok := slogtest.FindFirstError(entry)
	if !ok {
		return false
	}
	// Canceled queries usually happen when we're shutting down tests, and so
	// ignoring them should reduce flakiness.  This also includes
	// context.Canceled and context.DeadlineExceeded errors, even if they are
	// not part of a query.
	return isQueryCanceledError(err)
}

// isQueryCanceledError checks if the error is due to a query being canceled. This reproduces
// database.IsQueryCanceledError, but is reimplemented here to avoid importing the database package,
// which would result in import loops. We also use string matching on the error PostgreSQL returns
// to us, rather than the pq error type, because we don't want testutil, which is imported in lots
// of places to import lib/pq, which we have our own fork of.
func isQueryCanceledError(err error) bool {
	if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if strings.Contains(err.Error(), "canceling statement due to user request") {
		return true
	}
	return false
}
