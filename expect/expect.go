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
	"bytes"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

// Expectf reads from the Console's tty until the provided formatted string
// is read or an error occurs, and returns the buffer read by Console.
func (c *Console) Expectf(format string, args ...interface{}) (string, error) {
	return c.Expect(String(fmt.Sprintf(format, args...)))
}

// ExpectString reads from Console's tty until the provided string is read or
// an error occurs, and returns the buffer read by Console.
func (c *Console) ExpectString(s string) (string, error) {
	return c.Expect(String(s))
}

// ExpectEOF reads from Console's tty until EOF or an error occurs, and returns
// the buffer read by Console.  We also treat the PTSClosed error as an EOF.
func (c *Console) ExpectEOF() (string, error) {
	return c.Expect(EOF, PTSClosed)
}

// Expect reads from Console's tty until a condition specified from opts is
// encountered or an error occurs, and returns the buffer read by console.
// No extra bytes are read once a condition is met, so if a program isn't
// expecting input yet, it will be blocked. Sends are queued up in tty's
// internal buffer so that the next Expect will read the remaining bytes (i.e.
// rest of prompt) as well as its conditions.
func (c *Console) Expect(opts ...ExpectOpt) (string, error) {
	var options ExpectOpts
	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return "", err
		}
	}

	buf := new(bytes.Buffer)
	writer := io.MultiWriter(append(c.opts.Stdouts, buf)...)
	runeWriter := bufio.NewWriterSize(writer, utf8.UTFMax)

	readTimeout := c.opts.ReadTimeout
	if options.ReadTimeout != nil {
		readTimeout = options.ReadTimeout
	}

	var matcher Matcher
	var err error

	defer func() {
		for _, observer := range c.opts.ExpectObservers {
			if matcher != nil {
				observer([]Matcher{matcher}, buf.String(), err)
				return
			}
			observer(options.Matchers, buf.String(), err)
		}
	}()

	for {
		if readTimeout != nil {
			err = c.passthroughPipe.SetReadDeadline(time.Now().Add(*readTimeout))
			if err != nil {
				return buf.String(), err
			}
		}

		var r rune
		r, _, err = c.runeReader.ReadRune()
		if err != nil {
			matcher = options.Match(err)
			if matcher != nil {
				err = nil
				break
			}
			return buf.String(), err
		}

		c.Logf("expect read: %q", string(r))
		_, err = runeWriter.WriteRune(r)
		if err != nil {
			return buf.String(), err
		}

		// Immediately flush rune to the underlying writers.
		err = runeWriter.Flush()
		if err != nil {
			return buf.String(), err
		}

		matcher = options.Match(buf)
		if matcher != nil {
			break
		}
	}

	if matcher != nil {
		cb, ok := matcher.(CallbackMatcher)
		if ok {
			err = cb.Callback(buf)
			if err != nil {
				return buf.String(), err
			}
		}
	}

	return buf.String(), err
}
