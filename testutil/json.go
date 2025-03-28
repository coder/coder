package testutil

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// RequireJSONEq is like assert.RequireJSONEq, but it's actually readable.
// Note that this calls t.Fatalf under the hood, so it should never
// be called in a goroutine.
func RequireJSONEq(t *testing.T, expected, actual string) {
	t.Helper()

	var expectedJSON, actualJSON any
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		t.Fatalf("failed to unmarshal expected JSON: %s", err)
	}
	if err := json.Unmarshal([]byte(actual), &actualJSON); err != nil {
		t.Fatalf("failed to unmarshal actual JSON: %s", err)
	}

	if diff := cmp.Diff(expectedJSON, actualJSON); diff != "" {
		t.Fatalf("JSON diff (-want +got):\n%s", diff)
	}
}
