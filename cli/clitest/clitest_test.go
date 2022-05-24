package clitest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCli(t *testing.T) {
	t.Parallel()
	clitest.CreateTemplateVersionSource(t, nil)
	client := coderdtest.New(t, nil)
	cmd, config := clitest.New(t)
	clitest.SetupConfig(t, client, config)
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	go func() {
		_ = cmd.Execute()
	}()
	pty.ExpectMatch("coder")
}
