package terraform

import (
	"bufio"
	"bytes"
	"slices"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func ParseTimingLines(t *testing.T, input []byte) []*proto.Timing {
	t.Helper()

	// Parse the input into *proto.Timing structs.
	var expected []*proto.Timing
	scanner := bufio.NewScanner(bytes.NewBuffer(input))
	for scanner.Scan() {
		line := scanner.Bytes()

		var msg proto.Timing
		require.NoError(t, protojson.Unmarshal(line, &msg))

		expected = append(expected, &msg)
	}
	require.NoError(t, scanner.Err())
	StableSortTimings(t, expected) // To reduce flakiness.

	return expected
}

func TimingsAreEqual(t *testing.T, expected []*proto.Timing, actual []*proto.Timing) bool {
	t.Helper()

	// Shortcut check.
	if len(expected)+len(actual) == 0 {
		t.Log("both timings are empty")
		return true
	}

	// Shortcut check.
	if len(expected) != len(actual) {
		t.Logf("timings lengths are not equal: %d != %d", len(expected), len(actual))
		return false
	}

	// Compare each element; both are expected to be sorted in a stable manner.
	for i := 0; i < len(expected); i++ {
		ex := expected[i]
		ac := actual[i]
		if !protobuf.Equal(ex, ac) {
			t.Logf("timings are not equivalent: %q != %q", ex.String(), ac.String())
			return false
		}
	}

	return true
}

func PrintTiming(t *testing.T, timing *proto.Timing) {
	t.Helper()

	marshaler := protojson.MarshalOptions{
		Multiline: false, // Ensure it's set to false for single-line JSON
		Indent:    "",    // No indentation
	}

	out, err := marshaler.Marshal(timing)
	assert.NoError(t, err)
	t.Logf("%s", out)
}

func StableSortTimings(t *testing.T, timings []*proto.Timing) {
	t.Helper()

	slices.SortStableFunc(timings, func(a, b *proto.Timing) int {
		if a == nil || b == nil || a.Start == nil || b.Start == nil {
			return 0
		}

		if a.Start.AsTime().Equal(b.Start.AsTime()) {
			// Special case: when start times are equal, we need to keep the ordering stable, so we hash both entries
			// and sort based on that (since end times could be equal too, in principle).
			ah := xxhash.Sum64String(a.String())
			bh := xxhash.Sum64String(b.String())

			if ah == bh {
				// WTF.
				PrintTiming(t, a)
				PrintTiming(t, b)
				t.Fatalf("identical timings detected?!")
				return 0
			}

			if ah < bh {
				return -1
			}

			return 1
		}

		if a.Start.AsTime().Before(b.Start.AsTime()) {
			return -1
		}

		return 1
	})
}
