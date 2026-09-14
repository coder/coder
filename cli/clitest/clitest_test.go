package clitest_test

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestCli(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	clitest.CreateTemplateVersionSource(t, nil)
	client := coderdtest.New(t, nil)
	i, config := clitest.New(t)
	clitest.SetupConfig(t, client, config)
	stdout := expecter.NewAttachedToInvocation(t, i)
	clitest.Start(t, i)
	stdout.ExpectMatch(ctx, "coder")
}
