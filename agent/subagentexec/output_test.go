package subagentexec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRedactionToken stands in for a child agent token: long enough to
// straddle any flush boundary, with no self-similar structure.
const testRedactionToken = "s3cr3t-0f4b8c1d-2e6a-47f9-b0d5-9c8e7a6b5f43"

// minTokenFragment is the shortest piece of the token that is treated as a
// leak. Anything longer than a few bytes would let two records be pasted
// back into the token.
const minTokenFragment = 4

// writeAndCollect feeds writes through a writer that never truncates, closes
// it, and returns the emitted records.
func writeAndCollect(t *testing.T, secret string, writes ...string) []string {
	t.Helper()

	var records []string
	w := newRedactingWriter(secret, 1<<20, func(record string) {
		records = append(records, record)
	})
	for _, chunk := range writes {
		n, err := w.Write([]byte(chunk))
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	w.Close()
	return records
}

// requireRedacted checks that the emitted records reconstruct want and that
// no single record carries a reusable piece of the token. Comparing the
// concatenation is what proves the token was replaced rather than split.
func requireRedacted(t *testing.T, records []string, token, want string) {
	t.Helper()

	require.Equal(t, want, strings.Join(records, ""))
	for i, record := range records {
		require.NotContains(t, record, token, "record %d carries the whole token", i)
		for k := minTokenFragment; k < len(token); k++ {
			require.NotContains(t, record, token[:k],
				"record %d carries a %d byte token prefix", i, k)
			require.NotContains(t, record, token[len(token)-k:],
				"record %d carries a %d byte token suffix", i, k)
		}
		require.LessOrEqual(t, len(record), maxLogLineBytes, "record %d exceeds the size bound", i)
	}
}

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

// TestRedactingWriter_SplitAcrossWrites walks the token through every split
// position across two writes. A writer that only redacted whole writes would
// publish both halves.
func TestRedactingWriter_SplitAcrossWrites(t *testing.T) {
	t.Parallel()

	const (
		before = "token="
		after  = " done"
	)
	input := before + testRedactionToken + after
	want := before + redactedPlaceholder + after

	for split := 0; split <= len(input); split++ {
		t.Run(fmt.Sprintf("At%d", split), func(t *testing.T) {
			t.Parallel()
			records := writeAndCollect(t, testRedactionToken, input[:split], input[split:])
			requireRedacted(t, records, testRedactionToken, want)
		})
	}
}

// TestRedactingWriter_SplitByteAtATime covers the extreme case of one byte
// per write, which is what a driver would do to defeat write-scoped
// redaction.
func TestRedactingWriter_SplitByteAtATime(t *testing.T) {
	t.Parallel()

	input := "token=" + testRedactionToken + " done\n"
	writes := make([]string, 0, len(input))
	for i := range len(input) {
		writes = append(writes, input[i:i+1])
	}

	records := writeAndCollect(t, testRedactionToken, writes...)
	requireRedacted(t, records, testRedactionToken, "token="+redactedPlaceholder+" done")
}

// TestRedactingWriter_SplitAcrossForcedFlush is the regression test for the
// forced flush at the size bound: an unterminated line longer than the bound
// is emitted in chunks, and a token straddling a chunk boundary must not be
// emitted in halves.
func TestRedactingWriter_SplitAcrossForcedFlush(t *testing.T) {
	t.Parallel()

	const tail = "yyyy"
	// Every offset that places part of the token on each side of the bound,
	// plus the offsets just outside that window.
	first := maxLogLineBytes - len(testRedactionToken) - 2
	last := maxLogLineBytes + 2

	for start := first; start <= last; start++ {
		t.Run(fmt.Sprintf("TokenAt%d", start), func(t *testing.T) {
			t.Parallel()

			filler := strings.Repeat("x", start)
			input := filler + testRedactionToken + tail
			want := filler + redactedPlaceholder + tail

			// One write, so the split can only come from the size bound.
			records := writeAndCollect(t, testRedactionToken, input)
			require.Greater(t, len(records), 1, "the size bound must have forced a flush")
			requireRedacted(t, records, testRedactionToken, want)

			// The same input arriving in single bytes must redact
			// identically.
			writes := make([]string, 0, len(input))
			for i := range len(input) {
				writes = append(writes, input[i:i+1])
			}
			requireRedacted(t, writeAndCollect(t, testRedactionToken, writes...), testRedactionToken, want)
		})
	}
}

// TestRedactingWriter_MultipleOccurrences covers several occurrences in one
// record and across records.
func TestRedactingWriter_MultipleOccurrences(t *testing.T) {
	t.Parallel()

	t.Run("SameLine", func(t *testing.T) {
		t.Parallel()
		input := testRedactionToken + " and " + testRedactionToken + " and " + testRedactionToken + "\n"
		records := writeAndCollect(t, testRedactionToken, input)
		require.Len(t, records, 1)
		requireRedacted(t, records, testRedactionToken,
			redactedPlaceholder+" and "+redactedPlaceholder+" and "+redactedPlaceholder)
	})

	t.Run("AcrossLines", func(t *testing.T) {
		t.Parallel()
		records := writeAndCollect(t, testRedactionToken,
			"one "+testRedactionToken+"\ntwo "+testRedactionToken+"\n")
		require.Len(t, records, 2)
		requireRedacted(t, records, testRedactionToken,
			"one "+redactedPlaceholder+"two "+redactedPlaceholder)
	})

	t.Run("AroundForcedFlush", func(t *testing.T) {
		t.Parallel()
		filler := strings.Repeat("x", maxLogLineBytes/2)
		input := filler + testRedactionToken + filler + testRedactionToken + filler
		want := filler + redactedPlaceholder + filler + redactedPlaceholder + filler
		requireRedacted(t, writeAndCollect(t, testRedactionToken, input), testRedactionToken, want)
	})
}

// TestRedactingWriter_BoundedOnLongInput pins that a driver writing a long
// unterminated blob is emitted in bounded records and leaves a bounded
// buffer behind.
func TestRedactingWriter_BoundedOnLongInput(t *testing.T) {
	t.Parallel()

	var records []string
	w := newRedactingWriter(testRedactionToken, 1<<20, func(record string) {
		records = append(records, record)
	})

	blob := strings.Repeat("x", 10*maxLogLineBytes)
	for range 4 {
		n, err := w.Write([]byte(blob))
		require.NoError(t, err)
		require.Equal(t, len(blob), n)
		// Bounded memory: the writer never holds more than the hard bound,
		// no matter how large a single write is.
		require.LessOrEqual(t, w.buf.Len(), maxPendingBytes)
	}
	w.Close()

	require.Equal(t, 4*len(blob), len(strings.Join(records, "")))
	for i, record := range records {
		require.LessOrEqual(t, len(record), maxLogLineBytes, "record %d exceeds the size bound", i)
	}
}

// TestRedactingWriter_EmptySecret pins that a writer without a token to
// protect passes output through and still bounds it.
func TestRedactingWriter_EmptySecret(t *testing.T) {
	t.Parallel()

	records := writeAndCollect(t, "", "plain output\n")
	require.Equal(t, []string{"plain output"}, records)

	long := strings.Repeat("x", maxLogLineBytes+10)
	records = writeAndCollect(t, "", long)
	require.Equal(t, []string{long[:maxLogLineBytes], long[maxLogLineBytes:]}, records)
}

// TestRedactingWriter_SecretLongerThanLineBound covers the pathological case
// of a secret longer than the size bound: the writer stays bounded and still
// never emits the secret.
func TestRedactingWriter_SecretLongerThanLineBound(t *testing.T) {
	t.Parallel()

	secret := strings.Repeat("z", maxLogLineBytes+100)
	var records []string
	w := newRedactingWriter(secret, 1<<20, func(record string) {
		records = append(records, record)
	})

	n, err := w.Write([]byte(strings.Repeat("z", 4*maxLogLineBytes)))
	require.NoError(t, err)
	require.Equal(t, 4*maxLogLineBytes, n)
	require.LessOrEqual(t, w.buf.Len(), maxPendingBytes)
	w.Close()

	require.NotEmpty(t, records)
	for _, record := range records {
		require.NotContains(t, record, secret)
		require.LessOrEqual(t, len(record), maxLogLineBytes)
	}
}
