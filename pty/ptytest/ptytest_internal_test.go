package ptytest

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdbuf(t *testing.T) {
	t.Parallel()

	var got bytes.Buffer

	b := newStdbuf()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := io.Copy(&got, b)
		assert.NoError(t, err)
	}()

	_, err := b.Write([]byte("hello "))
	require.NoError(t, err)
	_, err = b.Write([]byte("world\n"))
	require.NoError(t, err)
	_, err = b.Write([]byte("bye\n"))
	require.NoError(t, err)

	err = b.Close()
	require.NoError(t, err)
	<-done

	assert.Equal(t, "hello world\nbye\n", got.String())
}
