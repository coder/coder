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
	"bufio"
	"io"
	"strings"
	"testing"
)

// NewTestConsole returns a new Console that multiplexes the application's
// stdout to go's testing logger. Primarily so that outputs from parallel tests
// using t.Parallel() is not interleaved.
func NewTestConsole(t *testing.T, opts ...ConsoleOpt) (*Console, error) {
	tf, err := NewTestWriter(t)
	if err != nil {
		return nil, err
	}

	return NewConsole(append(opts, WithStdout(tf))...)
}

// NewTestWriter returns an io.Writer where bytes written to the file are
// logged by go's testing logger. Bytes are flushed to the logger on line end.
func NewTestWriter(t *testing.T) (io.Writer, error) {
	r, w := io.Pipe()
	tw := testWriter{t}

	go func() {
		defer r.Close()

		br := bufio.NewReader(r)

		for {
			line, _, err := br.ReadLine()
			if err != nil {
				return
			}

			_, err = tw.Write(line)
			if err != nil {
				return
			}
		}
	}()

	return w, nil
}

// testWriter provides a io.Writer interface to go's testing logger.
type testWriter struct {
	t *testing.T
}

func (tw testWriter) Write(p []byte) (n int, err error) {
	tw.t.Log(string(p))
	return len(p), nil
}

// StripTrailingEmptyLines returns a copy of s stripped of trailing lines that
// consist of only space characters.
func StripTrailingEmptyLines(out string) string {
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return out
	}

	for i := len(lines) - 1; i >= 0; i-- {
		stripped := strings.Replace(lines[i], " ", "", -1)
		if len(stripped) == 0 {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}

	return strings.Join(lines, "\n")
}
