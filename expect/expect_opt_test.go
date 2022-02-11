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
	"bytes"
	"errors"
	"io"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpectOptString(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			String(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			String("Hello"),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			String("other", "world"),
			"Hello world",
			true,
		},
		{
			"No matches",
			String("hello"),
			"Hello world",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			buf := new(bytes.Buffer)
			_, err = buf.WriteString(test.data)
			require.Nil(t, err)

			matcher := options.Match(buf)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptRegexp(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			Regexp(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			Regexp(regexp.MustCompile(`^Hello`)),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			Regexp(regexp.MustCompile(`^Hello$`), regexp.MustCompile(`world$`)),
			"Hello world",
			true,
		},
		{
			"No matches",
			Regexp(regexp.MustCompile(`^Hello$`)),
			"Hello world",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			buf := new(bytes.Buffer)
			_, err = buf.WriteString(test.data)
			require.Nil(t, err)

			matcher := options.Match(buf)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptRegexpPattern(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No args",
			RegexpPattern(),
			"Hello world",
			false,
		},
		{
			"Single arg",
			RegexpPattern(`^Hello`),
			"Hello world",
			true,
		},
		{
			"Multiple arg",
			RegexpPattern(`^Hello$`, `world$`),
			"Hello world",
			true,
		},
		{
			"No matches",
			RegexpPattern(`^Hello$`),
			"Hello world",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			buf := new(bytes.Buffer)
			_, err = buf.WriteString(test.data)
			require.Nil(t, err)

			matcher := options.Match(buf)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptError(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     error
		expected bool
	}{
		{
			"No args",
			Error(),
			io.EOF,
			false,
		},
		{
			"Single arg",
			Error(io.EOF),
			io.EOF,
			true,
		},
		{
			"Multiple arg",
			Error(io.ErrShortWrite, io.EOF),
			io.EOF,
			true,
		},
		{
			"No matches",
			Error(io.ErrShortWrite),
			io.EOF,
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			matcher := options.Match(test.data)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptThen(t *testing.T) {
	var (
		errFirst  = errors.New("first")
		errSecond = errors.New("second")
	)

	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		match    bool
		expected error
	}{
		{
			"Noop",
			String("Hello").Then(func(buf *bytes.Buffer) error {
				return nil
			}),
			"Hello world",
			true,
			nil,
		},
		{
			"Short circuit",
			String("Hello").Then(func(buf *bytes.Buffer) error {
				return errFirst
			}).Then(func(buf *bytes.Buffer) error {
				return errSecond
			}),
			"Hello world",
			true,
			errFirst,
		},
		{
			"Chain",
			String("Hello").Then(func(buf *bytes.Buffer) error {
				return nil
			}).Then(func(buf *bytes.Buffer) error {
				return errSecond
			}),
			"Hello world",
			true,
			errSecond,
		},
		{
			"No matches",
			String("other").Then(func(buf *bytes.Buffer) error {
				return errFirst
			}),
			"Hello world",
			false,
			nil,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			buf := new(bytes.Buffer)
			_, err = buf.WriteString(test.data)
			require.Nil(t, err)

			matcher := options.Match(buf)
			if test.match {
				require.NotNil(t, matcher)

				cb, ok := matcher.(CallbackMatcher)
				if ok {
					require.True(t, ok)

					err = cb.Callback(nil)
					require.Equal(t, test.expected, err)
				}
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}

func TestExpectOptAll(t *testing.T) {
	tests := []struct {
		title    string
		opt      ExpectOpt
		data     string
		expected bool
	}{
		{
			"No opts",
			All(),
			"Hello world",
			true,
		},
		{
			"Single string match",
			All(String("Hello")),
			"Hello world",
			true,
		},
		{
			"Single string no match",
			All(String("Hello")),
			"No match",
			false,
		},
		{
			"Ordered strings match",
			All(String("Hello"), String("world")),
			"Hello world",
			true,
		},
		{
			"Ordered strings not all match",
			All(String("Hello"), String("world")),
			"Hello",
			false,
		},
		{
			"Unordered strings",
			All(String("world"), String("Hello")),
			"Hello world",
			true,
		},
		{
			"Unordered strings not all match",
			All(String("world"), String("Hello")),
			"Hello",
			false,
		},
		{
			"Repeated strings match",
			All(String("Hello"), String("Hello")),
			"Hello world",
			true,
		},
		{
			"Mixed opts match",
			All(String("Hello"), RegexpPattern(`wo[a-z]{1}ld`)),
			"Hello woxld",
			true,
		},
		{
			"Mixed opts no match",
			All(String("Hello"), RegexpPattern(`wo[a-z]{1}ld`)),
			"Hello wo4ld",
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			var options ExpectOpts
			err := test.opt(&options)
			require.Nil(t, err)

			buf := new(bytes.Buffer)
			_, err = buf.WriteString(test.data)
			require.Nil(t, err)

			matcher := options.Match(buf)
			if test.expected {
				require.NotNil(t, matcher)
			} else {
				require.Nil(t, matcher)
			}
		})
	}
}
