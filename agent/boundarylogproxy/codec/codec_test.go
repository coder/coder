package codec_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  codec.Tag
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := codec.WriteFrame(&buf, tt.tag, tt.data)
			require.NoError(t, err)

			readBuf := make([]byte, codec.MaxMessageSizeV1)
			tag, data, err := codec.ReadFrame(&buf, readBuf)
			require.NoError(t, err)
			require.Equal(t, tt.tag, tag)
			require.Equal(t, tt.data, data)
		})
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	t.Parallel()

	// Hand construct a header that indicates the message size exceeds the maximum
	// message size for codec.TagV1 by one. We just write the header to buf because
	// we expect codec.ReadFrame to bail out when reading the invalid length.
	header := uint32(codec.TagV1)<<codec.DataLength | (codec.MaxMessageSizeV1 + 1)
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, header)

	var buf bytes.Buffer
	_, err := buf.Write(data)
	require.NoError(t, err)

	readBuf := make([]byte, 1)
	_, _, err = codec.ReadFrame(&buf, readBuf)
	require.ErrorIs(t, err, codec.ErrMessageTooLarge)
}

func TestReadFrameEmptyReader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	readBuf := make([]byte, codec.MaxMessageSizeV1)
	_, _, err := codec.ReadFrame(&buf, readBuf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read header error")
}

func TestReadFrameInvalidTag(t *testing.T) {
	t.Parallel()

	// Hand construct a header that indicates a tag we don't know about. We just
	// write the header to buf because we expect codec.ReadFrame to bail out when
	// reading the invalid tag.
	const (
		dataLength uint32 = 10
		bogusTag   uint32 = 2
	)
	header := bogusTag<<codec.DataLength | dataLength
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, header)

	var buf bytes.Buffer
	_, err := buf.Write(data)
	require.NoError(t, err)

	readBuf := make([]byte, 1)
	_, _, err = codec.ReadFrame(&buf, readBuf)
	require.Error(t, err)
}

func TestReadFrameAllocatesWhenNeeded(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := []byte("this message is longer than the buffer")
	err := codec.WriteFrame(&buf, codec.TagV1, data)
	require.NoError(t, err)

	// Buffer with insufficient capacity triggers allocation.
	readBuf := make([]byte, 4)
	tag, got, err := codec.ReadFrame(&buf, readBuf)
	require.NoError(t, err)
	require.Equal(t, codec.TagV1, tag)
	require.Equal(t, data, got)
}

func TestWriteFrameDataSize(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	data := make([]byte, codec.MaxMessageSizeV1)
	err := codec.WriteFrame(&buf, codec.TagV1, data)
	require.NoError(t, err)

	//nolint: makezero // This intentionally increases the slice length.
	data = append(data, 0) // One byte over the maximum
	err = codec.WriteFrame(&buf, codec.TagV1, data)
	require.Error(t, err)
}

func TestWriteFrameInvalidTag(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	readBuf := make([]byte, 1)
	const bogusTag = 2
	err := codec.WriteFrame(&buf, codec.Tag(bogusTag), readBuf)
	require.Error(t, err)
}
