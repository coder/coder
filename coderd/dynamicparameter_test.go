package coderd_test

import (
	_ "embed"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	provProto "github.com/coder/coder/v2/provisionerd/proto"
)

func TestDynamicParameterTemplate(t *testing.T) {
	dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/dynamic/main.tf")
	require.NoError(t, err)

	setupDynamicParamsTest(t, setupDynamicParamsTestParams{
		provisionerDaemonVersion: provProto.CurrentVersion.String(),
		mainTF:                   dynamicParametersTerraformSource,
	})
}
