package clog_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clog"
)

func TestError(t *testing.T) {
	t.Parallel()
	t.Run("oneline", func(t *testing.T) {
		t.Parallel()
		var mockErr error = clog.Error("fake error")
		mockErr = xerrors.Errorf("wrap 1: %w", mockErr)
		mockErr = xerrors.Errorf("wrap 2: %w", mockErr)

		var buf bytes.Buffer
		clog.Log(&buf, mockErr)

		output, err := ioutil.ReadAll(&buf)
		require.NoError(t, err, "read all stderr output")
		require.Equal(t, "error: "+clog.Bold("fake error")+"\r\n\n", string(output), "output is as expected")
	})

	t.Run("plain-error", func(t *testing.T) {
		t.Parallel()
		mockErr := xerrors.Errorf("base error")
		mockErr = xerrors.Errorf("wrap 1: %w", mockErr)

		var buf bytes.Buffer
		clog.Log(&buf, mockErr)

		output, err := ioutil.ReadAll(&buf)
		require.NoError(t, err, "read all stderr output")

		require.Equal(t, "fatal: "+clog.Bold("wrap 1: base error")+"\r\n\n", string(output), "output is as expected")
	})

	t.Run("message", func(t *testing.T) {
		for _, f := range []struct {
			f     func(io.Writer, string, ...string)
			level string
		}{{clog.LogInfo, "info"}, {clog.LogSuccess, "success"}, {clog.LogWarn, "warning"}} {
			var buf bytes.Buffer
			f.f(&buf, "testing", clog.Hintf("maybe do %q", "this"), clog.BlankLine, clog.Causef("what happened was %q", "this"))

			output, err := ioutil.ReadAll(&buf)
			require.NoError(t, err, "read all stderr output")

			require.Equal(t, f.level+": "+clog.Bold("testing")+"\r\n  | "+clog.Bold("hint:")+" maybe do \"this\"\r\n  | \r\n  | "+clog.Bold("cause:")+" what happened was \"this\"\r\n", string(output), "output is as expected")
		}
	})

	t.Run("multi-line", func(t *testing.T) {
		var mockErr error = clog.Error("fake header", "next line", clog.BlankLine, clog.Tipf("content of fake tip"))
		mockErr = xerrors.Errorf("wrap 1: %w", mockErr)
		mockErr = xerrors.Errorf("wrap 1: %w", mockErr)

		var buf bytes.Buffer
		clog.Log(&buf, mockErr)

		output, err := ioutil.ReadAll(&buf)
		require.NoError(t, err, "read all stderr output")

		require.Equal(t, "error: "+clog.Bold("fake header")+"\r\n  | next line\r\n  | \r\n  | "+clog.Bold("tip:")+" content of fake tip\r\n\n", string(output), "output is as expected")
	})
}
