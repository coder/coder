package codec_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  uint8
		data []byte
	}{
		{
			name: "empty data",
			tag:  codec.TagV1,
			data: []byte{},
		},
		{
			name: "simple data",
			tag:  codec.TagV1,
			data: []byte("hello world"),
		},
		{
			name: "binary data",
			tag:  codec.TagV1,
			data: []byte{0x00, 0x01, 0x02, 0xff, 0xfe},
		},
		{
			name: "max tag value",
			tag:  15,
			data: []byte("use all the tag bits"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := codec.WriteFrame(&buf, tt.tag, tt.data)
			require.NoError(t, err)

			readBuf := make([]byte, codec.MaxMessageSize)
			tag, data, err := codec.ReadFrame(&buf, codec.MaxMessageSize, readBuf)
			require.NoError(t, err)
			require.Equal(t, tt.tag, tag)
			require.Equal(t, tt.data, data)
		})
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := make([]byte, 1000)
	err := codec.WriteFrame(&buf, codec.TagV1, data)
	require.NoError(t, err)

	readBuf := make([]byte, 100)
	_, _, err = codec.ReadFrame(&buf, 100, readBuf)
	require.ErrorIs(t, err, codec.ErrTooLarge)
}

func TestReadFrameEmptyReader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	readBuf := make([]byte, codec.MaxMessageSize)
	_, _, err := codec.ReadFrame(&buf, codec.MaxMessageSize, readBuf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read header error")
}

func TestReadFrameAllocatesWhenNeeded(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := []byte("this message is longer than the buffer")
	err := codec.WriteFrame(&buf, codec.TagV1, data)
	require.NoError(t, err)

	// Buffer with insufficient capacity triggers allocation.
	readBuf := make([]byte, 4)
	tag, got, err := codec.ReadFrame(&buf, codec.MaxMessageSize, readBuf)
	require.NoError(t, err)
	require.Equal(t, uint8(codec.TagV1), tag)
	require.Equal(t, data, got)
}
