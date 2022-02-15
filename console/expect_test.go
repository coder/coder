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
	"bufio"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"golang.org/x/xerrors"

	. "github.com/coder/coder/console"
)

var (
	ErrWrongAnswer = xerrors.New("wrong answer")
)

type Survey struct {
	Prompt string
	Answer string
}

func Prompt(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	for _, survey := range []Survey{
		{
			"What is 1+1?", "2",
		},
		{
			"What is Netflix backwards?", "xilfteN",
		},
	} {
		_, err := fmt.Fprintf(out, "%s: ", survey.Prompt)
		if err != nil {
			return err
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		_, err = fmt.Fprint(out, text)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text != survey.Answer {
			return ErrWrongAnswer
		}
	}

	return nil
}

func newTestConsole(t *testing.T, opts ...Opt) (*Console, error) {
	opts = append([]Opt{
		expectNoError(t),
	}, opts...)
	return NewConsole(opts...)
}

func expectNoError(t *testing.T) Opt {
	return WithExpectObserver(
		func(matchers []Matcher, buf string, err error) {
			if err == nil {
				return
			}
			if len(matchers) == 0 {
				t.Fatalf("Error occurred while matching %q: %s\n%s", buf, err, string(debug.Stack()))
			} else {
				var criteria []string
				for _, matcher := range matchers {
					criteria = append(criteria, fmt.Sprintf("%q", matcher.Criteria()))
				}
				t.Fatalf("Failed to find [%s] in %q: %s\n%s", strings.Join(criteria, ", "), buf, err, string(debug.Stack()))
			}
		},
	)
}

func testCloser(t *testing.T, closer io.Closer) {
	if err := closer.Close(); err != nil {
		t.Errorf("Close failed: %s", err)
		debug.PrintStack()
	}
}

func TestExpectf(t *testing.T) {
	t.Parallel()

	console, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, console)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		console.Expectf("What is 1+%d?", 1)
		console.SendLine("2")
		console.Expectf("What is %s backwards?", "Netflix")
		console.SendLine("xilfteN")
	}()

	err = Prompt(console.InTty(), console.OutTty())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	wg.Wait()
}

func TestExpect(t *testing.T) {
	t.Parallel()

	console, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, console)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		console.ExpectString("What is 1+1?")
		console.SendLine("2")
		console.ExpectString("What is Netflix backwards?")
		console.SendLine("xilfteN")
	}()

	err = Prompt(console.InTty(), console.OutTty())
	if err != nil {
		t.Errorf("Expected no error but got '%s'", err)
	}
	wg.Wait()
}

func TestExpectOutput(t *testing.T) {
	t.Parallel()

	console, err := newTestConsole(t)
	if err != nil {
		t.Errorf("Expected no error but got'%s'", err)
	}
	defer testCloser(t, console)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		console.ExpectString("What is 1+1?")
		console.SendLine("3")
	}()

	err = Prompt(console.InTty(), console.OutTty())
	if err == nil || !errors.Is(err, ErrWrongAnswer) {
		t.Errorf("Expected error '%s' but got '%s' instead", ErrWrongAnswer, err)
	}
	wg.Wait()
}
