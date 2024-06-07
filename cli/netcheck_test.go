package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestNetcheck(t *testing.T) {
	t.Parallel()

	pty := ptytest.New(t)
	config := login(t, pty)

	var out bytes.Buffer
	inv, _ := clitest.New(t, "netcheck", "--global-config", string(config))
	inv.Stdout = &out

	clitest.StartWithWaiter(t, inv).RequireSuccess()

	b := out.Bytes()
	t.Log(string(b))
	var report healthsdk.DERPHealthReport
	require.NoError(t, json.Unmarshal(b, &report))

	// We do not assert that the report is healthy, just that
	// it has the expected number of reports per region.
	require.Len(t, report.Regions, 1+1) // 1 built-in region + 1 test-managed STUN region
	for _, v := range report.Regions {
		require.Len(t, v.NodeReports, len(v.Region.Nodes))
	}
}
