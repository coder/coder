package deployment_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/deployment"
)

func TestFlags(t *testing.T) {
	t.Parallel()

	df := deployment.Flags()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	deployment.AttachFlags(fs, df, false)

	require.NotNil(t, fs.Lookup("access-url"))
	require.False(t, fs.Lookup("access-url").Hidden)
	require.True(t, fs.Lookup("telemetry-url").Hidden)
	require.NotEmpty(t, fs.Lookup("telemetry-url").DefValue)
	require.Nil(t, fs.Lookup("audit-logging"))

	df = deployment.Flags()
	fs = pflag.NewFlagSet("test-enterprise", pflag.ContinueOnError)
	deployment.AttachFlags(fs, df, true)

	require.Nil(t, fs.Lookup("access-url"))
	require.NotNil(t, fs.Lookup("audit-logging"))
	require.Contains(t, fs.Lookup("audit-logging").Usage, "This is an Enterprise feature")
}
