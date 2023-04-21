package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestUserCreate(t *testing.T) {
	t.Parallel()
	t.Run("Prompts", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "users", "create")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()
		matches := []string{
			"Username", "dean",
			"Email", "dean@coder.com",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		<-doneChan
	})
}
