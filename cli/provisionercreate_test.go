package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestProvisionerCreate(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "provisioners", "create", "foobar")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		err := cmd.Execute()
		require.NoError(t, err)

		var token *uuid.UUID
		const tokenPrefix = "coder provisioners run --token "
		s := bufio.NewScanner(buf)
		for s.Scan() {
			line := s.Text()
			if strings.HasPrefix(line, tokenPrefix) {
				tokenString := strings.TrimPrefix(line, tokenPrefix)
				parsedToken, err := uuid.Parse(tokenString)
				require.NoError(t, err, "provisioner token has invalid format")
				token = &parsedToken
			}
		}
		require.NotNil(t, token, "provisioner token not generated in output")

		provisioners, err := client.ProvisionerDaemons(context.Background())
		require.NoError(t, err)
		tokensByName := make(map[string]*uuid.UUID)
		for _, p := range provisioners {
			tokensByName[p.Name] = p.AuthToken
		}
		require.Equal(t, token, tokensByName["foobar"])
	})

	t.Run("Unprivileged", func(t *testing.T) {
		t.Parallel()
		adminClient := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, adminClient)
		otherClient := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
		cmd, root := clitest.New(t, "provisioners", "create", "foobar")
		clitest.SetupConfig(t, otherClient, root)
		err := cmd.Execute()
		require.Error(t, err, "unprivileged user was allowed to create provisioner")
	})
}
