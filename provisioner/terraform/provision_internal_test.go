package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestProvisionEnvSetsAWSPRMUserAgent(t *testing.T) {
	t.Parallel()

	env, err := provisionEnv(&proto.Config{}, &proto.Metadata{}, nil, nil, nil)
	require.NoError(t, err)
	require.Contains(t, env, "AWS_SDK_UA_APP_ID=APN_1.1/pc_cdfmjwn8i6u8l9fwz8h82e4w3$")
}
