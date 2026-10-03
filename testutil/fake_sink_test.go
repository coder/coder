package testutil_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestFakeSink(t *testing.T) {
	t.Parallel()

	t.Run("BasicCapture", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Debug(ctx, "first test message")
		logger.Debug(ctx, "second test message")
		logger.Debug(ctx, "third test message")

		entries := sink.Entries()
		require.Len(t, entries, 3)
	})

	t.Run("FilterByLevel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Debug(ctx, "debug level message")
		logger.Info(ctx, "info level message")
		logger.Error(ctx, "error level message")

		errorOnly := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError
		})
		require.Len(t, errorOnly, 1)
		assert.Equal(t, "error level message", errorOnly[0].Message)
	})

	t.Run("MultipleFiltersAND", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Info(ctx, "hello world filter test")
		logger.Info(ctx, "goodbye world filter")
		logger.Error(ctx, "hello error filter test")

		byLevel := func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelInfo
		}
		byMessage := func(e slog.SinkEntry) bool {
			return strings.Contains(e.Message, "hello")
		}

		matched := sink.Entries(byLevel, byMessage)
		require.Len(t, matched, 1)
		assert.Equal(t, "hello world filter test", matched[0].Message)
	})

	t.Run("NilFilterSkipped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Info(ctx, "nil filter test msg")

		// A nil filter should be harmlessly skipped.
		entries := sink.Entries(nil)
		require.Len(t, entries, 1)
	})

	t.Run("NoFilters", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Debug(ctx, "no filter debug msg")
		logger.Info(ctx, "no filter info msg")

		entries := sink.Entries()
		require.Len(t, entries, 2)
	})

	t.Run("EmptySink", func(t *testing.T) {
		t.Parallel()

		sink := testutil.NewFakeSink(t)
		entries := sink.Entries()
		assert.Empty(t, entries)
	})

	t.Run("ThreadSafety", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		const goroutines = 10
		const entriesPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(goroutines)
		for range goroutines {
			go func() {
				defer wg.Done()
				for range entriesPerGoroutine {
					logger.Debug(ctx, "concurrent log entry")
				}
			}()
		}
		wg.Wait()

		entries := sink.Entries()
		require.Len(t, entries, goroutines*entriesPerGoroutine)
	})

	t.Run("LoggerConvenience", func(t *testing.T) {
		t.Parallel()

		t.Run("DefaultDebug", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			sink := testutil.NewFakeSink(t)
			logger := sink.Logger()

			logger.Debug(ctx, "captured at debug level")

			entries := sink.Entries()
			require.Len(t, entries, 1)
			assert.Equal(t, slog.LevelDebug, entries[0].Level)
		})

		t.Run("RespectsLevel", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			sink := testutil.NewFakeSink(t)
			logger := sink.Logger(slog.LevelInfo)

			// Debug should be filtered out by the logger
			// because the level is set to Info.
			logger.Debug(ctx, "filtered out by level")
			logger.Info(ctx, "kept by info level")

			entries := sink.Entries()
			require.Len(t, entries, 1)
			assert.Equal(t, slog.LevelInfo, entries[0].Level)
		})
	})
}
