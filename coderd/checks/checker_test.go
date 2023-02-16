package checks_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/checks"
	"github.com/coder/coder/testutil"
)

func Test_Checker(t *testing.T) {
	t.Parallel()
	var (
		tick    = make(chan time.Time)
		c       = checks.New(tick, slogtest.Make(t, nil))
		done    = make(chan struct{})
		testErr = assert.AnError
	)

	t.Cleanup(func() { c.Stop() })

	c.Add("good-test", func() error {
		done <- struct{}{}
		return nil
	})
	c.Add("bad-test", func() error {
		done <- struct{}{}
		return testErr
	})
	c.Add("slow-test", func() error {
		<-time.After(testutil.WaitShort)
		done <- struct{}{}
		return nil
	})

	go func() {
		tick <- time.Now()
	}()

	<-done // first check
	<-done // second check
	results := c.Results()
	assert.Len(t, results, 2)
	assert.Empty(t, results["good-test"].Error)
	assert.Equal(t, results["bad-test"].Error, testErr.Error())
	assert.Nil(t, results["slow-test"])
	<-done // third check
	results = c.Results()
	assert.Len(t, results, 3)
	assert.Empty(t, results["slow-test"].Error)
}
