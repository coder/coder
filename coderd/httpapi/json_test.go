package httpapi_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestDuration(t *testing.T) {
	t.Parallel()

	t.Run("MarshalJSON", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			value    time.Duration
			expected string
		}{
			{
				value:    0,
				expected: "0s",
			},
			{
				value:    1 * time.Millisecond,
				expected: "1ms",
			},
			{
				value:    1 * time.Second,
				expected: "1s",
			},
			{
				value:    1 * time.Minute,
				expected: "1m0s",
			},
			{
				value:    1 * time.Hour,
				expected: "1h0m0s",
			},
			{
				value:    1*time.Hour + 1*time.Minute + 1*time.Second + 1*time.Millisecond,
				expected: "1h1m1.001s",
			},
		}

		for _, c := range cases {
			t.Run(c.expected, func(t *testing.T) {
				t.Parallel()

				d := httpapi.Duration(c.value)
				b, err := d.MarshalJSON()
				require.NoError(t, err)
				require.Equal(t, `"`+c.expected+`"`, string(b))
			})
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			value    string
			expected time.Duration
		}{
			{
				value:    "0ms",
				expected: 0,
			},
			{
				value:    "0s",
				expected: 0,
			},
			{
				value:    "1ms",
				expected: 1 * time.Millisecond,
			},
			{
				value:    "1s",
				expected: 1 * time.Second,
			},
			{
				value:    "1m",
				expected: 1 * time.Minute,
			},
			{
				value:    "1m0s",
				expected: 1 * time.Minute,
			},
			{
				value:    "1h",
				expected: 1 * time.Hour,
			},
			{
				value:    "1h0m0s",
				expected: 1 * time.Hour,
			},
			{
				value:    "1h1m1.001s",
				expected: 1*time.Hour + 1*time.Minute + 1*time.Second + 1*time.Millisecond,
			},
			{
				value:    "1h1m1s1ms",
				expected: 1*time.Hour + 1*time.Minute + 1*time.Second + 1*time.Millisecond,
			},
		}

		for _, c := range cases {
			t.Run(c.value, func(t *testing.T) {
				t.Parallel()

				var d httpapi.Duration
				err := d.UnmarshalJSON([]byte(`"` + c.value + `"`))
				require.NoError(t, err)
				require.Equal(t, c.expected, time.Duration(d))
			})
		}
	})

	t.Run("UnmarshalJSONInt", func(t *testing.T) {
		t.Parallel()

		var d httpapi.Duration
		err := d.UnmarshalJSON([]byte("12345"))
		require.NoError(t, err)
		require.EqualValues(t, 12345, d)
	})

	t.Run("UnmarshalJSONErrors", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			value       string
			errContains string
		}{
			{
				value:       "not valid json (no double quotes)",
				errContains: "unmarshal JSON value",
			},
			{
				value:       `"not valid duration"`,
				errContains: "parse duration",
			},
			{
				value:       "{}",
				errContains: "invalid duration",
			},
		}

		for _, c := range cases {
			t.Run(c.value, func(t *testing.T) {
				t.Parallel()

				var d httpapi.Duration
				err := d.UnmarshalJSON([]byte(c.value))
				require.Error(t, err)
				require.Contains(t, err.Error(), c.errContains)
			})
		}
	})
}
