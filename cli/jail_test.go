package cli_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestJail(t *testing.T) {
	t.Parallel()
	t.Run("Basic Usage", func(t *testing.T) {
		t.Parallel()

		// coder jail requires root privileges to run
		if os.Getgid() != 0 {
			t.Skip("skipped jail test because it requires root")
		}

		inv, _ := clitest.New(t, "jail", "--allow", "pastebin.com", "--", "curl", "https://pastebin.com/raw/2q6kyAyQ")
		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()

		pty.ExpectMatch("hello")

		cancelFunc()
		<-done
	})
}
