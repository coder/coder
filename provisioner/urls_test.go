package provisioner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner"
)

func TestValidateExternalURL(t *testing.T) {
	t.Parallel()

	validURLs := []string{
		"https://coder.com",
		"https://coder.com/docs/code-server",
		"http://localhost:3000",
		"http://127.0.0.1:8080/path?query=1#frag",
		"zed://ssh/coder.dev",
		"vscode://coder.coder-remote/open?owner=me",
		"jetbrains-gateway://connect#type=coder",
		"coder://dev.coder.com/v0/open/ws/dev/agent/main/rdp",
	}

	invalidURLs := []string{
		"",
		"coder.com",
		"coder.com/docs",
		"/relative/path",
		"//coder.com",
		"https://",
		"https://:8080",
		"mailto:support@coder.com",
		"file:///home/coder",
		"https://coder.com/\x7f",
	}

	for _, value := range validURLs {
		t.Run("Valid/"+value, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, provisioner.ValidateExternalURL(value))
		})
	}

	for _, value := range invalidURLs {
		t.Run("Invalid/"+value, func(t *testing.T) {
			t.Parallel()
			require.Error(t, provisioner.ValidateExternalURL(value))
		})
	}
}

func TestValidateExternalURLEmpty(t *testing.T) {
	t.Parallel()

	err := provisioner.ValidateExternalURL("")
	require.EqualError(t, err, "must include a scheme and host")
}
