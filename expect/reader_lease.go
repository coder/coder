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
	"context"
	"io"

	"golang.org/x/xerrors"
)

// ReaderLease provides cancellable io.Readers from an underlying io.Reader.
type ReaderLease struct {
	reader io.Reader
	bytec  chan byte
}

// NewReaderLease returns a new ReaderLease that begins reading the given
// io.Reader.
func NewReaderLease(reader io.Reader) *ReaderLease {
	readerLease := &ReaderLease{
		reader: reader,
		bytec:  make(chan byte),
	}

	go func() {
		for {
			bytes := make([]byte, 1)
			n, err := readerLease.reader.Read(bytes)
			if err != nil {
				return
			}
			if n == 0 {
				panic("non eof read 0 bytes")
			}
			readerLease.bytec <- bytes[0]
		}
	}()

	return readerLease
}

// NewReader returns a cancellable io.Reader for the underlying io.Reader.
// Readers can be canceled without interrupting other Readers, and once
// a reader is a canceled it will not read anymore bytes from ReaderLease's
// underlying io.Reader.
func (rm *ReaderLease) NewReader(ctx context.Context) io.Reader {
	return NewChanReader(ctx, rm.bytec)
}

type chanReader struct {
	ctx   context.Context
	bytec <-chan byte
}

// NewChanReader returns a io.Reader over a byte chan. If context is canceled,
// future Reads will return io.EOF.
func NewChanReader(ctx context.Context, bytec <-chan byte) io.Reader {
	return &chanReader{
		ctx:   ctx,
		bytec: bytec,
	}
}

func (cr *chanReader) Read(bytes []byte) (n int, err error) {
	select {
	case <-cr.ctx.Done():
		return 0, io.EOF
	case b := <-cr.bytec:
		if len(bytes) < 1 {
			return 0, xerrors.Errorf("cannot read into 0 len byte slice")
		}
		bytes[0] = b
		return 1, nil
	}
}
