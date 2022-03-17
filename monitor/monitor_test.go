package monitor

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cryptorand"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/xerrors"
)

func TestContext(t *testing.T) {
	t.Parallel()

	t.Run("DefaultMonitor", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		emptyM := FromContext(ctx)
		require.Equal(t, &DefaultMonitor, emptyM, "no monitor attached returns DefaultMonitor")
	})

	t.Run("ExtractFromContext", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		m := New(MonitorOptions{})
		newCtx := WithMonitor(ctx, m)
		newMon := FromContext(newCtx)
		require.Equal(t, m, newMon, "WithMonitor attaches monitor to context")
		require.NotEqual(t, ctx, newCtx, "returned context is a copy of given context")
	})

	t.Run("Copy", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		m := New(MonitorOptions{})
		newCtx := WithMonitor(ctx, m)
		newMon := FromContext(newCtx)

		cancelCtx, cancel := context.WithCancel(newCtx)
		defer cancel()
		cancelMon := FromContext(cancelCtx)
		require.Equal(t, newMon, cancelMon, "copyied context still has monitor attached")
	})
}

func TestTrace(t *testing.T) {
	t.Parallel()

	t.Run("ChildSpan", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		m := New(MonitorOptions{
			TracerProvider: trace.NewTracerProvider(),
		})
		newCtx := WithMonitor(ctx, m)
		name1, _ := cryptorand.String(10)
		name2, _ := cryptorand.String(10)

		end1 := Trace(&newCtx, name1)
		s1 := Span(newCtx)
		require.True(t, s1.IsRecording(), "span1 is recording")

		end2 := Trace(&newCtx, name2)
		s2 := Span(newCtx)
		require.True(t, s1.IsRecording(), "span1 is recording after creating child")
		require.True(t, s2.IsRecording(), "span2 is recording")

		end2()
		require.False(t, s2.IsRecording(), "span2 ended")

		require.True(t, s1.IsRecording(), "span1 is still recording after child span ended")
		end1()
		require.False(t, s1.IsRecording(), "span1 ended")
	})
}

func TestLogger(t *testing.T) {
	t.Parallel()

	t.Run("Debug", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		Debug(ctx, msg)
		require.Contains(t, b.String(), msg, "debug log contains message")
	})

	t.Run("Info", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		Info(ctx, msg)
		require.Contains(t, b.String(), msg, "info log contains message")
	})

	t.Run("Warn", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		Warn(ctx, msg)
		require.Contains(t, b.String(), msg, "warn log contains message")
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		Error(ctx, msg, xerrors.New(msg))
		require.Contains(t, b.String(), msg, "error log contains message")
	})

	t.Run("Critical", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		Critical(ctx, msg)
		require.Contains(t, b.String(), msg, "critical log contains message")
	})

	t.Run("LogNamed", func(t *testing.T) {
		t.Parallel()

		var (
			b bytes.Buffer
			m = New(MonitorOptions{
				Logger: slog.Make(sloghuman.Sink(&b)).Leveled(slog.LevelDebug),
			})
		)

		ctx := WithMonitor(context.Background(), m)
		msg, _ := cryptorand.String(10)
		n1, _ := cryptorand.String(10)
		n2, _ := cryptorand.String(10)
		LogNamed(ctx, n1)
		Info(ctx, msg)
		require.Contains(t, b.String(), n1, "log contains name")
		LogNamed(ctx, n2)
		Info(ctx, msg)
		require.Contains(t, b.String(), fmt.Sprintf("%s.%s", n1, n2), "log contains concat name")
	})

}
