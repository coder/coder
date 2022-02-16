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

package console

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"unicode/utf8"

	"github.com/coder/coder/console/pty"
)

// Console is an interface to automate input and output for interactive
// applications. Console can block until a specified output is received and send
// input back on it's tty. Console can also multiplex other sources of input
// and multiplex its output to other writers.
type Console struct {
	opts       Opts
	pty        pty.Pty
	runeReader *bufio.Reader
	closers    []io.Closer
}

// Opt allows setting Console options.
type Opt func(*Opts) error

// Opts provides additional options on creating a Console.
type Opts struct {
	Logger          *log.Logger
	Stdouts         []io.Writer
	ExpectObservers []Observer
}

// Observer provides an interface for a function callback that will
// be called after each Expect operation.
// matchers will be the list of active matchers when an error occurred,
//   or a list of matchers that matched `buf` when err is nil.
// buf is the captured output that was matched against.
// err is error that might have occurred. May be nil.
type Observer func(matchers []Matcher, buf string, err error)

// WithStdout adds writers that Console duplicates writes to, similar to the
// Unix tee(1) command.
//
// Each write is written to each listed writer, one at a time. Console is the
// last writer, writing to it's internal buffer for matching expects.
// If a listed writer returns an error, that overall write operation stops and
// returns the error; it does not continue down the list.
func WithStdout(writers ...io.Writer) Opt {
	return func(opts *Opts) error {
		opts.Stdouts = append(opts.Stdouts, writers...)
		return nil
	}
}

// WithLogger adds a logger for Console to log debugging information to. By
// default Console will discard logs.
func WithLogger(logger *log.Logger) Opt {
	return func(opts *Opts) error {
		opts.Logger = logger
		return nil
	}
}

// WithExpectObserver adds an ExpectObserver to allow monitoring Expect operations.
func WithExpectObserver(observers ...Observer) Opt {
	return func(opts *Opts) error {
		opts.ExpectObservers = append(opts.ExpectObservers, observers...)
		return nil
	}
}

// NewConsole returns a new Console with the given options.
func NewConsole(opts ...Opt) (*Console, error) {
	options := Opts{
		Logger: log.New(ioutil.Discard, "", 0),
	}

	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return nil, err
		}
	}

	consolePty, err := pty.New()
	if err != nil {
		return nil, err
	}
	closers := []io.Closer{consolePty}
	reader := consolePty.Reader()

	cons := &Console{
		opts:       options,
		pty:        consolePty,
		runeReader: bufio.NewReaderSize(reader, utf8.UTFMax),
		closers:    closers,
	}

	return cons, nil
}

// Tty returns an input Tty for accepting input
func (c *Console) InTty() *os.File {
	return c.pty.InPipe()
}

// OutTty returns an output tty for writing
func (c *Console) OutTty() *os.File {
	return c.pty.OutPipe()
}

// Close closes Console's tty. Calling Close will unblock Expect and ExpectEOF.
func (c *Console) Close() error {
	for _, fd := range c.closers {
		err := fd.Close()
		if err != nil {
			c.Logf("failed to close: %s", err)
		}
	}
	return nil
}

// Send writes string s to Console's tty.
func (c *Console) Send(s string) (int, error) {
	c.Logf("console send: %q", s)
	n, err := c.pty.WriteString(s)
	return n, err
}

// SendLine writes string s to Console's tty with a trailing newline.
func (c *Console) SendLine(s string) (int, error) {
	bytes, err := c.Send(fmt.Sprintf("%s\n", s))

	return bytes, err
}

// Log prints to Console's logger.
// Arguments are handled in the manner of fmt.Print.
func (c *Console) Log(v ...interface{}) {
	c.opts.Logger.Print(v...)
}

// Logf prints to Console's logger.
// Arguments are handled in the manner of fmt.Printf.
func (c *Console) Logf(format string, v ...interface{}) {
	c.opts.Logger.Printf(format, v...)
}
