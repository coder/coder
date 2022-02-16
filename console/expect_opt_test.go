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

package console_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/coder/coder/console"
)

func TestExpectOptString(t *testing.T) {
	t.Parallel()

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
		test := test
		t.Run(test.title, func(t *testing.T) {
			t.Parallel()

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

func TestExpectOptAll(t *testing.T) {
	t.Parallel()

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
	}

	for _, test := range tests {
		test := test
		t.Run(test.title, func(t *testing.T) {
			t.Parallel()
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
