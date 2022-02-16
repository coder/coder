package clitest_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/console"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCli(t *testing.T) {
	t.Parallel()
	clitest.CreateProjectVersionSource(t, nil)
	client := coderdtest.New(t)
	cmd, config := clitest.New(t)
	clitest.SetupConfig(t, client, config)
	cons := console.New(t, cmd)
	go func() {
		err := cmd.Execute()
		require.NoError(t, err)
	}()
	_, err := cons.ExpectString("coder")
	require.NoError(t, err)
}
