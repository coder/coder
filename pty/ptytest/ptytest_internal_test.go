package ptytest

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStdbuf(t *testing.T) {
	t.Parallel()

	var got bytes.Buffer

	b := newStdbuf()
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(&got, b)
	}()

	b.Write([]byte("hello "))
	b.Write([]byte("world\n"))
	b.Write([]byte("bye\n"))

	b.Close()
	<-done

	assert.Equal(t, "hello world\nbye\n", got.String())
}
