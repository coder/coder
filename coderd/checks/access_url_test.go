package checks_test

import (
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/checks"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_CanHitAccessURL(t *testing.T) {
	t.Parallel()

	var (
		ch      = make(chan time.Time)
		done    = make(chan struct{})
		log     = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		checker = checks.New(ch, log)
		client  = coderdtest.New(t, &coderdtest.Options{
			Checker: checker,
		})
	)

	checker.Add("access-url", checks.CanHitAccessURL(client.URL, testutil.WaitShort))
	go func() {
		checker.Run()
		close(done)
	}()
	go func() {
		ch <- time.Now()
		<-time.After(testutil.WaitShort)
		checker.Stop()
	}()
	<-done
	results := checker.Results()
	assert.Len(t, results, 1)
	assert.Empty(t, results["access-url"].Error)
}
