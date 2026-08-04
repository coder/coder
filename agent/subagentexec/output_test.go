package subagentexec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactingWriter(t *testing.T) {
	t.Parallel()

	collect := func(lines *[]string) func(string) {
		return func(line string) { *lines = append(*lines, line) }
	}

	t.Run("RedactsExactSecret", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{secret: "s3cr3t", maxLines: 16}
		w.emit = collect(&lines)

		_, err := w.Write([]byte("token=s3cr3t tail\nplain\n"))
		require.NoError(t, err)
		require.Equal(t, []string{"token=" + redactedPlaceholder + " tail", "plain"}, lines)
	})

	t.Run("RedactsSecretSplitAcrossWrites", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{secret: "s3cr3t", maxLines: 16}
		w.emit = collect(&lines)

		// A driver that writes a token one byte at a time still cannot get
		// it into the log: lines are only emitted once complete.
		for _, chunk := range []string{"tok", "en=s3", "cr", "3t\n"} {
			_, err := w.Write([]byte(chunk))
			require.NoError(t, err)
		}
		require.Equal(t, []string{"token=" + redactedPlaceholder}, lines)
	})

	t.Run("FlushesPartialLineOnClose", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{secret: "s3cr3t", maxLines: 16}
		w.emit = collect(&lines)

		_, err := w.Write([]byte("no newline s3cr3t"))
		require.NoError(t, err)
		require.Empty(t, lines)

		w.Close()
		require.Equal(t, []string{"no newline " + redactedPlaceholder}, lines)

		// Output after close is dropped rather than logged against a
		// finished execution.
		_, err = w.Write([]byte("late\n"))
		require.NoError(t, err)
		w.Close()
		require.Len(t, lines, 1)
	})

	t.Run("StripsCarriageReturn", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{maxLines: 16}
		w.emit = collect(&lines)

		_, err := w.Write([]byte("windows\r\n"))
		require.NoError(t, err)
		require.Equal(t, []string{"windows"}, lines)
	})

	t.Run("BoundsEmittedLines", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{maxLines: 2}
		w.emit = collect(&lines)

		_, err := w.Write([]byte("one\ntwo\nthree\nfour\n"))
		require.NoError(t, err)
		require.Equal(t, []string{
			"one",
			"two",
			"subagent execution driver output truncated: line limit reached",
		}, lines)
	})

	t.Run("BoundsLineLength", func(t *testing.T) {
		t.Parallel()
		var lines []string
		w := &redactingWriter{maxLines: 16}
		w.emit = collect(&lines)

		long := make([]byte, maxLogLineBytes+10)
		for i := range long {
			long[i] = 'x'
		}
		_, err := w.Write(long)
		require.NoError(t, err)
		require.Len(t, lines, 1)
		require.Len(t, lines[0], maxLogLineBytes)

		w.Close()
		require.Len(t, lines, 2)
		require.Len(t, lines[1], 10)
	})

	t.Run("DefaultsLineBound", func(t *testing.T) {
		t.Parallel()
		w := newRedactingWriter("secret", 0, func(string) {})
		require.Equal(t, maxDriverOutputLines, w.maxLines)
	})
}
