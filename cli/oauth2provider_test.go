package cli_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2ProviderDCR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		command     string
		expectValue bool
		expectMsg   string
	}{
		{
			name:        "Enable",
			command:     "enable",
			expectValue: true,
			expectMsg:   "Dynamic client registration is now enabled.",
		},
		{
			name:        "Disable",
			command:     "disable",
			expectValue: false,
			expectMsg:   "Dynamic client registration is now disabled.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)

			inv, root := clitest.New(t, "oauth2-provider", "dcr", tt.command)
			clitest.SetupConfig(t, client, root)

			var buf bytes.Buffer
			inv.Stderr = &buf
			err := inv.Run()
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tt.expectMsg)

			ctx := testutil.Context(t, testutil.WaitShort)
			settings, err := client.OAuth2ProviderSettings(ctx)
			require.NoError(t, err)
			require.NotNil(t, settings.DynamicClientRegistrationEnabled, "GET must always return a concrete value")
			require.Equal(t, tt.expectValue, *settings.DynamicClientRegistrationEnabled)
		})
	}
}

func TestOAuth2ProviderDCR_RegularUser(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	anotherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	inv, root := clitest.New(t, "oauth2-provider", "dcr", "enable")
	clitest.SetupConfig(t, anotherClient, root)

	var buf bytes.Buffer
	inv.Stderr = &buf
	err := inv.Run()
	var sdkError *codersdk.Error
	require.Error(t, err)
	require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
	assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
}
