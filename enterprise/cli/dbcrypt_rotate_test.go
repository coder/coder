package cli_test

import (
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/stretchr/testify/require"
)

func TestDBCryptRotate(t *testing.T) {
	t.Parallel()

	// TODO: create a test database and populate some encrypted data with cipher A

	client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureExternalTokenEncryption: 1,
		},
	}})

	// Run the cmd with ciphers B,A

	inv, conf := newCLI(t, "dbcrypt-rotate") // TODO: env?
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	clitest.SetupConfig(t, client, conf)

	err := inv.Run()
	require.NoError(t, err)

	// TODO: validate that all data has been updated with the checksum of the new cipher.
}
