package cron_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule/cron"
)

// standardVectors is the shape of testdata/standard_vectors.json. Times are RFC
// 3339 with an offset. next lists the first runs after the time in after, each
// written with the offset of the zone at that instant. reason tells a person why
// Standard rejects a spec. The test checks that it is there and not its text, so
// that rewording an error does not change the contract.
type standardVectors struct {
	Specs []struct {
		Spec     string `json:"spec"`
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	} `json:"specs"`
	Runs []struct {
		Name     string   `json:"name"`
		Spec     string   `json:"spec"`
		TimeZone string   `json:"timeZone"`
		After    string   `json:"after"`
		Next     []string `json:"next"`
	} `json:"runs"`
}

// TestStandard_Vectors checks each entry of testdata/standard_vectors.json, the
// specs that Standard accepts and rejects and the runs that Next gives around
// daylight saving changes, against cron.Standard. The file is the contract with
// any other implementation of these schedules, such as the browser preview of an
// automation's next runs. A test of the preview can read the same file, so a
// difference between the two fails CI on one side.
func TestStandard_Vectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "standard_vectors.json"))
	require.NoError(t, err)
	var vectors standardVectors
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&vectors))
	require.NotEmpty(t, vectors.Specs)
	require.NotEmpty(t, vectors.Runs)

	t.Run("Specs", func(t *testing.T) {
		t.Parallel()
		for _, tc := range vectors.Specs {
			// Whether a spec is valid does not depend on the zone.
			sched, err := cron.Standard(tc.Spec, "UTC")
			if tc.Accepted {
				require.NoErrorf(t, err, "spec %q", tc.Spec)
				continue
			}
			require.Errorf(t, err, "spec %q", tc.Spec)
			require.Nilf(t, sched, "spec %q", tc.Spec)
			require.NotEmptyf(t, tc.Reason, "spec %q", tc.Spec)
		}
	})

	t.Run("Runs", func(t *testing.T) {
		t.Parallel()
		for _, tc := range vectors.Runs {
			t.Run(tc.Name, func(t *testing.T) {
				t.Parallel()
				require.NotEmpty(t, tc.Next)
				sched, err := cron.Standard(tc.Spec, tc.TimeZone)
				require.NoError(t, err)
				after := mustParseTime(t, time.RFC3339, tc.After).In(mustLocation(t, tc.TimeZone))
				require.Equal(t, tc.Next, nextN(t, sched, after, len(tc.Next)))
			})
		}
	})
}
