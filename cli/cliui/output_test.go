package cliui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

type format struct {
	id              string
	attachOptionsFn func(opts *serpent.OptionSet)
	formatFn        func(ctx context.Context, data any) (string, error)
}

var _ cliui.OutputFormat = &format{}

func (f *format) ID() string {
	return f.id
}

func (f *format) AttachOptions(opts *serpent.OptionSet) {
	if f.attachOptionsFn != nil {
		f.attachOptionsFn(opts)
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
		require.NotPanics(t, func() {
			cliui.NewOutputFormatter(cliui.JSONFormat(), cliui.TextFormat())
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

		var called atomic.Int64
		f := cliui.NewOutputFormatter(
			cliui.JSONFormat(),
			&format{
				id: "foo",
				attachOptionsFn: func(opts *serpent.OptionSet) {
					opts.Add(serpent.Option{
						Name:          "foo",
						Flag:          "foo",
						FlagShorthand: "f",
						Value:         serpent.DiscardValue,
						Description:   "foo flag 1234",
					})
				},
				formatFn: func(_ context.Context, _ any) (string, error) {
					called.Add(1)
					return "foo", nil
				},
			},
		)

		cmd := &serpent.Command{}
		f.AttachOptions(&cmd.Options)

		fs := cmd.Options.FlagSet()

		selected := cmd.Options.ByFlag("output")
		require.NotNil(t, selected)
		require.Equal(t, "json", selected.Value.String())
		usage := fs.FlagUsages()
		require.Contains(t, usage, "Output format.")
		require.Contains(t, usage, "foo flag 1234")

		ctx := context.Background()
		data := []string{"hi", "dean", "was", "here"}
		out, err := f.Format(ctx, data)
		require.NoError(t, err)

		var got []string
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		require.Equal(t, data, got)
		require.EqualValues(t, 0, called.Load())

		require.NoError(t, fs.Set("output", "foo"))
		out, err = f.Format(ctx, data)
		require.NoError(t, err)
		require.Equal(t, "foo", out)
		require.EqualValues(t, 1, called.Load())

		require.Error(t, fs.Set("output", "bar"))
		out, err = f.Format(ctx, data)
		require.NoError(t, err)
		require.Equal(t, "foo", out)
		require.EqualValues(t, 2, called.Load())
	})

	t.Run("Write", func(t *testing.T) {
		t.Parallel()

		f := cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
		cmd := &serpent.Command{}
		f.AttachOptions(&cmd.Options)

		var buf bytes.Buffer
		require.NoError(t, f.Write(context.Background(), &buf, "hello"))
		require.Equal(t, "hello\n", buf.String())

		require.NoError(t, cmd.Options.FlagSet().Set("output", "json"))
		buf.Reset()
		require.NoError(t, f.Write(context.Background(), &buf, "hello"))
		require.Equal(t, "\"hello\"\n", buf.String())
	})

	t.Run("WriteFormatError", func(t *testing.T) {
		t.Parallel()

		f := cliui.NewOutputFormatter(
			cliui.TextFormatFunc(func(int) (string, error) { return "unreachable", nil }),
			cliui.JSONFormat(),
		)
		cmd := &serpent.Command{}
		f.AttachOptions(&cmd.Options)

		var buf bytes.Buffer
		require.Error(t, f.Write(context.Background(), &buf, "not an int"))
		require.Empty(t, buf.String())
	})
}

func Test_TextFormatFunc(t *testing.T) {
	t.Parallel()

	type item struct {
		Name string `json:"name"`
	}

	f := cliui.TextFormatFunc(func(i item) (string, error) {
		return strings.ToUpper(i.Name), nil
	})
	require.Equal(t, "text", f.ID())

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		out, err := f.Format(context.Background(), item{Name: "coder"})
		require.NoError(t, err)
		require.Equal(t, "CODER", out)
	})

	t.Run("WrongType", func(t *testing.T) {
		t.Parallel()

		_, err := f.Format(context.Background(), "coder")
		require.ErrorContains(t, err, "expected cliui_test.item, got string")
	})

	t.Run("FuncError", func(t *testing.T) {
		t.Parallel()

		failing := cliui.TextFormatFunc(func(item) (string, error) {
			return "", assert.AnError
		})
		_, err := failing.Format(context.Background(), item{})
		require.ErrorIs(t, err, assert.AnError)
	})
}
