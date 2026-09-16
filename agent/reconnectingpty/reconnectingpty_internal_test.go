package reconnectingpty

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestReadConnLoopSkipsEmptyInput(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		requests []workspacesdk.ReconnectingPTYRequest
		want     []string
		resizes  [][2]uint16
	}{
		{
			name:     "ResizeOnly",
			requests: []workspacesdk.ReconnectingPTYRequest{{Height: 40, Width: 100}},
			want:     []string{"resize"},
			resizes:  [][2]uint16{{40, 100}},
		},
		{
			name:     "EmptyRequestThenInput",
			requests: []workspacesdk.ReconnectingPTYRequest{{}, {Data: "input\r"}},
			want:     []string{"write:input\r"},
		},
		{
			name: "ResizeThenInput",
			requests: []workspacesdk.ReconnectingPTYRequest{
				{Height: 40, Width: 100}, {Data: "input\r"},
			},
			want:    []string{"resize", "write:input\r"},
			resizes: [][2]uint16{{40, 100}},
		},
		{
			name: "CombinedInputAndResize",
			requests: []workspacesdk.ReconnectingPTYRequest{
				{Data: "input\r", Height: 40, Width: 100}, {Data: "next\r"},
			},
			want:    []string{"write:input\r", "resize", "write:next\r"},
			resizes: [][2]uint16{{40, 100}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var input bytes.Buffer
			for _, req := range tt.requests {
				require.NoError(t, json.NewEncoder(&input).Encode(req))
			}
			conn := &testutil.ReaderWriterConn{Reader: &input, Writer: io.Discard}
			ptty := &recordingInputPTY{}
			metrics := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_pty_errors"}, []string{"kind"})
			readConnLoop(t.Context(), conn, ptty, metrics, slogtest.Make(t, nil))
			require.Zero(t, ptty.emptyWrites)
			require.Equal(t, tt.want, ptty.operations)
			require.Equal(t, tt.resizes, ptty.resizes)
		})
	}
}

type recordingInputPTY struct {
	emptyWrites int
	operations  []string
	resizes     [][2]uint16
}

func (*recordingInputPTY) Close() error { return nil }

func (p *recordingInputPTY) InputWriter() io.Writer { return p }

func (*recordingInputPTY) OutputReader() io.Reader { return bytes.NewReader(nil) }

func (p *recordingInputPTY) Resize(height, width uint16) error {
	p.operations = append(p.operations, "resize")
	p.resizes = append(p.resizes, [2]uint16{height, width})
	return nil
}

func (p *recordingInputPTY) Write(data []byte) (int, error) {
	if len(data) == 0 {
		p.emptyWrites++
		return 0, xerrors.New("empty PTY writes must be skipped")
	}
	p.operations = append(p.operations, "write:"+string(data))
	return len(data), nil
}

func TestWithTerminalEnv(t *testing.T) {
	t.Parallel()

	defaultLocale := "C.UTF-8"
	if runtime.GOOS == "darwin" {
		defaultLocale = "UTF-8"
	}

	tests := []struct {
		name           string
		env            []string
		wantLCCTYPE    string
		wantLCCTYPESet bool
	}{
		{
			name:           "adds locale when missing",
			env:            []string{"PATH=/bin"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name:           "adds locale when lang is not utf8",
			env:            []string{"LANG=C"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "keeps utf8 lang",
			env:  []string{"LANG=C.UTF-8"},
		},
		{
			name: "keeps unhyphenated utf8 lang",
			env:  []string{"LANG=C.UTF8"},
		},
		{
			name:           "keeps utf8 ctype",
			env:            []string{"LC_CTYPE=C.UTF-8"},
			wantLCCTYPE:    "C.UTF-8",
			wantLCCTYPESet: true,
		},
		{
			name:           "overrides non utf8 ctype",
			env:            []string{"LANG=C.UTF-8", "LC_CTYPE=C"},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "keeps utf8 lc all",
			env:  []string{"LC_ALL=C.UTF-8"},
		},
		{
			name: "preserves non empty lc all",
			env:  []string{"LC_ALL=C"},
		},
		{
			name:           "ignores empty lc all",
			env:            []string{"LC_ALL="},
			wantLCCTYPE:    defaultLocale,
			wantLCCTYPESet: true,
		},
		{
			name: "continues after empty lc all",
			env:  []string{"LC_ALL=", "LANG=C.UTF-8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := withTerminalEnv(tt.env)
			term, ok := envValue(got, "TERM")
			require.True(t, ok)
			require.Equal(t, xterm256Color, term)

			wantLCCTYPE := tt.wantLCCTYPE
			wantLCCTYPESet := tt.wantLCCTYPESet
			if runtime.GOOS == "windows" {
				wantLCCTYPE, wantLCCTYPESet = envValue(tt.env, "LC_CTYPE")
			}

			locale, ok := envValue(got, "LC_CTYPE")
			require.Equal(t, wantLCCTYPESet, ok)
			if wantLCCTYPESet {
				require.Equal(t, wantLCCTYPE, locale)
			}
		})
	}
}
