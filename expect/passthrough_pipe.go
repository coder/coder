// Copyright 2018 Netflix, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expect

import (
	"io"
	"os"
	"time"
)

// PassthroughPipe is pipes data from a io.Reader and allows setting a read
// deadline. If a timeout is reached the error is returned, otherwise the error
// from the provided io.Reader is returned is passed through instead.
type PassthroughPipe struct {
	reader *os.File
	errC   chan error
}

// NewPassthroughPipe returns a new pipe for a io.Reader that passes through
// non-timeout errors.
func NewPassthroughPipe(reader io.Reader) (*PassthroughPipe, error) {
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	errC := make(chan error, 1)
	go func() {
		defer close(errC)
		_, readerErr := io.Copy(pipeWriter, reader)
		if readerErr == nil {
			// io.Copy reads from reader until EOF, and a successful Copy returns
			// err == nil. We set it back to io.EOF to surface the error to Expect.
			readerErr = io.EOF
		}

		// Closing the pipeWriter will unblock the pipeReader.Read.
		err = pipeWriter.Close()
		if err != nil {
			// If we are unable to close the pipe, and the pipe isn't already closed,
			// the caller will hang indefinitely.
			panic(err)
			return
		}

		// When an error is read from reader, we need it to passthrough the err to
		// callers of (*PassthroughPipe).Read.
		errC <- readerErr
	}()

	return &PassthroughPipe{
		reader: pipeReader,
		errC:   errC,
	}, nil
}

func (pp *PassthroughPipe) Read(p []byte) (n int, err error) {
	n, err = pp.reader.Read(p)
	if err != nil {
		if os.IsTimeout(err) {
			return n, err
		}

		// If the pipe is closed, this is the second time calling Read on
		// PassthroughPipe, so just return the error from the os.Pipe io.Reader.
		perr, ok := <-pp.errC
		if !ok {
			return n, err
		}

		return n, perr
	}

	return n, nil
}

func (pp *PassthroughPipe) Close() error {
	return pp.reader.Close()
}

func (pp *PassthroughPipe) SetReadDeadline(t time.Time) error {
	return pp.reader.SetReadDeadline(t)
}
