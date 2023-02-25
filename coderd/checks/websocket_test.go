package checks_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/checks"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/testutil"
)

func Test_CanDialWebsocket(t *testing.T) {
	var (
		ch      = make(chan time.Time)
		done    = make(chan struct{})
		log     = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		checker = checks.New(ch, log)
		client  = coderdtest.New(t, &coderdtest.Options{
			Checker: checker,
		})
	)

	checker.Add("dial-websocket", checks.CanDialWebsocket(client.URL, testutil.WaitShort))
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
	assert.Empty(t, results["dial-websocket"].Error)
}
