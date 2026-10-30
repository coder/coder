package githubapp

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// CredentialFor builds the credential a stored GitHub App row mints tokens
// with. The API base comes from the MCP server's URL, so tokens are only
// minted for the GitHub the server talks to.
func CredentialFor(config database.MCPServerConfig, stored database.MCPServerGitHubAppCredential) (Credential, error) {
	if stored.MCPServerConfigID != config.ID {
		return Credential{}, xerrors.New("the GitHub App credential belongs to another MCP server")
	}
	apiBase, err := APIBaseURLFromServerURL(config.Url)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		APIBaseURL:     apiBase,
		AppID:          stored.AppID,
		InstallationID: stored.InstallationID,
		PrivateKeyPEM:  stored.PrivateKey,
		Repositories:   stored.Repositories,
	}, nil
}
