package cliui_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
)

type format struct {
	id            string
	attachFlagsFn func(cmd *cobra.Command)
	formatFn      func(ctx context.Context, data any) (string, error)
}

var _ cliui.OutputFormat = &format{}

func (f *format) ID() string {
	return f.id
}

func (f *format) AttachFlags(cmd *cobra.Command) {
	if f.attachFlagsFn != nil {
		f.attachFlagsFn(cmd)
	}
}

func (f *format) Format(ctx context.Context, data any) (string, error) {
	if f.formatFn != nil {
		return f.formatFn(ctx, data)
	}

	return "", nil
}

func Test_OutputFormatter(t *testing.T) {
	t.Parallel()

	t.Run("RequiresTwoFormatters", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			cliui.NewOutputFormatter()
		})
		require.Panics(t, func() {
			cliui.NewOutputFormatter(cliui.JSONFormat())
		})
	})

	t.Run("NoMissingFormatID", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			cliui.NewOutputFormatter(
				cliui.JSONFormat(),
				&format{id: ""},
			)
		})
	})

	t.Run("NoDuplicateFormats", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			cliui.NewOutputFormatter(
				cliui.JSONFormat(),
				cliui.JSONFormat(),
			)
		})
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var called int64
		f := cliui.NewOutputFormatter(
			cliui.JSONFormat(),
			&format{
				id: "foo",
				attachFlagsFn: func(cmd *cobra.Command) {
					cmd.Flags().StringP("foo", "f", "", "foo flag 1234")
				},
				formatFn: func(_ context.Context, _ any) (string, error) {
					atomic.AddInt64(&called, 1)
					return "foo", nil
				},
			},
		)

		cmd := &cobra.Command{}
		f.AttachFlags(cmd)

		selected, err := cmd.Flags().GetString("output")
		require.NoError(t, err)
		require.Equal(t, "json", selected)
		usage := cmd.Flags().FlagUsages()
		require.Contains(t, usage, "Available formats: json, foo")
		require.Contains(t, usage, "foo flag 1234")

		ctx := context.Background()
		data := []string{"hi", "dean", "was", "here"}
		out, err := f.Format(ctx, data)
		require.NoError(t, err)

		var got []string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		require.Equal(t, data, got)
		require.EqualValues(t, 0, atomic.LoadInt64(&called))

		require.NoError(t, cmd.Flags().Set("output", "foo"))
		out, err = f.Format(ctx, data)
		require.NoError(t, err)
		require.Equal(t, "foo", out)
		require.EqualValues(t, 1, atomic.LoadInt64(&called))

		require.NoError(t, cmd.Flags().Set("output", "bar"))
		out, err = f.Format(ctx, data)
		require.Error(t, err)
		require.ErrorContains(t, err, "bar")
		require.Equal(t, "", out)
		require.EqualValues(t, 1, atomic.LoadInt64(&called))
	})
}
