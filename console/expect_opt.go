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
	"strings"
	"time"
)

// Opt allows settings Expect options.
type Opt func(*Opts) error

// ConsoleCallback is a callback function to execute if a match is found for
// the chained matcher.
type ConsoleCallback func(buf *bytes.Buffer) error

// Opts provides additional options on Expect.
type Opts struct {
	Matchers    []Matcher
	ReadTimeout *time.Duration
}

// Match sequentially calls Match on all matchers in ExpectOpts and returns the
// first matcher if a match exists, otherwise nil.
func (eo Opts) Match(v interface{}) Matcher {
	for _, matcher := range eo.Matchers {
		if matcher.Match(v) {
			return matcher
		}
	}
	return nil
}

// CallbackMatcher is a matcher that provides a Callback function.
type CallbackMatcher interface {
	// Callback executes the matcher's callback with the content buffer at the
	// time of match.
	Callback(buf *bytes.Buffer) error
}

// Matcher provides an interface for finding a match in content read from
// Console's tty.
type Matcher interface {
	// Match returns true iff a match is found.
	Match(v interface{}) bool
	Criteria() interface{}
}

// stringMatcher fulfills the Matcher interface to match strings against a given
// bytes.Buffer.
type stringMatcher struct {
	str string
}

func (sm *stringMatcher) Match(v interface{}) bool {
	buf, ok := v.(*bytes.Buffer)
	if !ok {
		return false
	}
	if strings.Contains(buf.String(), sm.str) {
		return true
	}
	return false
}

func (sm *stringMatcher) Criteria() interface{} {
	return sm.str
}

// allMatcher fulfills the Matcher interface to match a group of ExpectOpt
// against any value.
type allMatcher struct {
	options Opts
}

func (am *allMatcher) Match(v interface{}) bool {
	var matchers []Matcher
	for _, matcher := range am.options.Matchers {
		if matcher.Match(v) {
			continue
		}
		matchers = append(matchers, matcher)
	}

	am.options.Matchers = matchers
	return len(matchers) == 0
}

func (am *allMatcher) Criteria() interface{} {
	var criteria []interface{}
	for _, matcher := range am.options.Matchers {
		criteria = append(criteria, matcher.Criteria())
	}
	return criteria
}

// All adds an Expect condition to exit if the content read from Console's tty
// matches all of the provided ExpectOpt, in any order.
func All(expectOpts ...Opt) Opt {
	return func(opts *Opts) error {
		var options Opts
		for _, opt := range expectOpts {
			if err := opt(&options); err != nil {
				return err
			}
		}

		opts.Matchers = append(opts.Matchers, &allMatcher{
			options: options,
		})
		return nil
	}
}

// String adds an Expect condition to exit if the content read from Console's
// tty contains any of the given strings.
func String(strs ...string) Opt {
	return func(opts *Opts) error {
		for _, str := range strs {
			opts.Matchers = append(opts.Matchers, &stringMatcher{
				str: str,
			})
		}
		return nil
	}
}
