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

		logger.Debug(ctx, "first")
		logger.Debug(ctx, "second")
		logger.Debug(ctx, "third")

		entries := sink.Entries()
		require.Len(t, entries, 3)
	})

	t.Run("FilterByLevel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Debug(ctx, "debug-msg")
		logger.Info(ctx, "info-msg")
		logger.Error(ctx, "error-msg")

		errorOnly := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError
		})
		require.Len(t, errorOnly, 1)
		assert.Equal(t, "error-msg", errorOnly[0].Message)
	})

	t.Run("MultipleFiltersAND", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Info(ctx, "hello world")
		logger.Info(ctx, "goodbye world")
		logger.Error(ctx, "hello error")

		byLevel := func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelInfo
		}
		byMessage := func(e slog.SinkEntry) bool {
			return strings.Contains(e.Message, "hello")
		}

		matched := sink.Entries(byLevel, byMessage)
		require.Len(t, matched, 1)
		assert.Equal(t, "hello world", matched[0].Message)
	})

	t.Run("NilFilterSkipped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Info(ctx, "msg")

		// A nil filter should be harmlessly skipped.
		entries := sink.Entries(nil)
		require.Len(t, entries, 1)
	})

	t.Run("NoFilters", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		logger.Debug(ctx, "one")
		logger.Info(ctx, "two")

		entries := sink.Entries()
		require.Len(t, entries, 2)
	})

	t.Run("EmptySink", func(t *testing.T) {
		t.Parallel()

		sink := testutil.NewFakeSink(t)
		entries := sink.Entries()
		assert.Nil(t, entries)
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
					logger.Debug(ctx, "concurrent")
				}
			}()
		}
		wg.Wait()

		entries := sink.Entries()
		require.Len(t, entries, goroutines*entriesPerGoroutine)
	})

	t.Run("NotifyChannel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		sink := testutil.NewFakeSink(t)
		ch := make(chan slog.SinkEntry, 1)
		sink.SetNotifyChannel(ch)
		logger := sink.Logger()

		logger.Info(ctx, "ping")

		select {
		case got := <-ch:
			assert.Equal(t, "ping", got.Message)
		case <-ctx.Done():
			t.Fatal("timed out waiting for notify channel")
		}
	})

	t.Run("LoggerConvenience", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		t.Run("DefaultDebug", func(t *testing.T) {
			t.Parallel()

			sink := testutil.NewFakeSink(t)
			logger := sink.Logger()

			logger.Debug(ctx, "captured")

			entries := sink.Entries()
			require.Len(t, entries, 1)
			assert.Equal(t, slog.LevelDebug, entries[0].Level)
		})

		t.Run("RespectsLevel", func(t *testing.T) {
			t.Parallel()

			sink := testutil.NewFakeSink(t)
			logger := sink.Logger(slog.LevelInfo)

			// Debug should be filtered out by the logger
			// because the level is set to Info.
			logger.Debug(ctx, "filtered-out")
			logger.Info(ctx, "kept")

			entries := sink.Entries()
			require.Len(t, entries, 1)
			assert.Equal(t, slog.LevelInfo, entries[0].Level)
		})
	})
}
