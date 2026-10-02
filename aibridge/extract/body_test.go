package extract_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/extract"
)

func TestDecodeBody(t *testing.T) {
	t.Parallel()

	gzipped := func(b []byte) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(b)
		_ = zw.Close()
		return buf.Bytes()
	}
	body := []byte(`{"id":"resp_1"}`)
	atLimit := bytes.Repeat([]byte("x"), extract.MaxBodyBytes)
	overLimit := bytes.Repeat([]byte("x"), extract.MaxBodyBytes+1)
	truncated := gzipped(body)
	truncated = truncated[:len(truncated)/2]

	cases := []struct {
		name     string
		encoding string
		raw      []byte
		want     []byte
		wantErr  string
	}{
		{name: "no_encoding", encoding: "", raw: body, want: body},
		{name: "identity", encoding: "identity", raw: body, want: body},
		{name: "identity_at_limit", encoding: "", raw: atLimit, want: atLimit},
		{name: "identity_over_limit", encoding: "", raw: overLimit, wantErr: "exceeds"},
		{name: "gzip", encoding: " GZIP ", raw: gzipped(body), want: body},
		{name: "x_gzip", encoding: "x-gzip", raw: gzipped(body), want: body},
		{name: "gzip_at_limit", encoding: "gzip", raw: gzipped(atLimit), want: atLimit},
		// The bound applies to the decoded size, so a small compressed
		// body cannot expand past it.
		{name: "gzip_over_limit", encoding: "gzip", raw: gzipped(overLimit), wantErr: "exceeds"},
		{name: "invalid_gzip", encoding: "gzip", raw: body, wantErr: "invalid gzip"},
		{name: "truncated_gzip", encoding: "gzip", raw: truncated, wantErr: "decode body"},
		{name: "unsupported_encoding", encoding: "br", raw: body, wantErr: "unsupported"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := extract.DecodeBody(tc.encoding, tc.raw)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
