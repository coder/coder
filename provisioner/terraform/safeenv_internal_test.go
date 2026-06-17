package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeEnvironValue(t *testing.T) {
	t.Parallel()

	env := []string{
		"FOO=bar",
		"AWS_SDK_UA_APP_ID=my-existing-id",
		"BAZ=qux",
	}
	require.Equal(t, "my-existing-id", safeEnvironValue(env, "AWS_SDK_UA_APP_ID"))
	require.Equal(t, "bar", safeEnvironValue(env, "FOO"))
	require.Equal(t, "", safeEnvironValue(env, "MISSING"))
}

func TestAWSSDKUserAgentEnv(t *testing.T) {
	t.Parallel()

	t.Run("NoExisting", func(t *testing.T) {
		t.Parallel()
		require.Equal(t,
			"AWS_SDK_UA_APP_ID=APN_1.1/pc_cdfmjwn8i6u8l9fwz8h82e4w3$",
			awsSDKUserAgentEnv(""),
		)
	})

	t.Run("AppendToExisting", func(t *testing.T) {
		t.Parallel()
		// When the operator is themselves an AWS Partner and has set their own
		// Application ID, we append Coder's with a space delimiter so both
		// attributions are preserved. See:
		// https://docs.aws.amazon.com/PRM/latest/aws-prm-onboarding-guide/automated-user-agent.html
		require.Equal(t,
			"AWS_SDK_UA_APP_ID=EXISTING_APP_ID APN_1.1/pc_cdfmjwn8i6u8l9fwz8h82e4w3$",
			awsSDKUserAgentEnv("EXISTING_APP_ID"),
		)
	})
}
