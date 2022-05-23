package clitest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	clitest.CreateTemplateVersionSource(t, nil)
	client := coderdtest.New(t, nil)
	cmd, config := clitest.New(t)
	clitest.SetupConfig(t, client, config)
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	errC := make(chan error)
	go func() {
		errC <- cmd.ExecuteContext(ctx)
	}()
	pty.ExpectMatch("coder")
	cancelFunc()
	require.NoError(t, <-errC)
}
