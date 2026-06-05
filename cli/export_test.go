package cli

import "testing"

// SSHWorkspaceSelectTestEnv is shared test fixture state for interactive SSH
// workspace selection tests in both cli and cli_test packages.
type SSHWorkspaceSelectTestEnv = sshWorkspaceSelectEnv

// SetupSSHWorkspaceSelectTestEnv provisions a user, organization, and database
// for interactive SSH workspace selection tests.
func SetupSSHWorkspaceSelectTestEnv(t *testing.T) SSHWorkspaceSelectTestEnv {
	return setupSSHWorkspaceSelectEnv(t)
}

// SSHDualAgentMutator returns a workspace agent mutator with two agents.
var SSHDualAgentMutator = sshDualAgentMutator

// SSHStopWorkspace stops the latest build for a workspace.
var SSHStopWorkspace = sshStopWorkspace

// SSHWorkspaceAgentToken returns the auth token for a named workspace agent.
var SSHWorkspaceAgentToken = sshWorkspaceAgentToken
