package workspaceapps

import (
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

type AccessMethod string

const (
	AccessMethodPath      AccessMethod = "path"
	AccessMethodSubdomain AccessMethod = "subdomain"
)

type Request struct {
	AccessMethod AccessMethod
	// BasePath of the app. For path apps, this is the path prefix in the router
	// for this particular app. For subdomain apps, this should be "/". This is
	// used for setting the cookie path.
	BasePath string

	UsernameOrID string
	// WorkspaceAndAgent xor WorkspaceNameOrID are required.
	WorkspaceAndAgent string // "workspace" or "workspace.agent"
	WorkspaceNameOrID string
	// AgentNameOrID is not required if the workspace has only one agent.
	AgentNameOrID string
	AppSlugOrPort string
}

func (r Request) Validate() error {
	if r.AccessMethod != AccessMethodPath && r.AccessMethod != AccessMethodSubdomain {
		return xerrors.Errorf("invalid access method: %q", r.AccessMethod)
	}
	if r.BasePath == "" {
		return xerrors.New("base path is required")
	}
	if r.UsernameOrID == "" {
		return xerrors.New("username or ID is required")
	}
	if r.UsernameOrID == codersdk.Me {
		// We block "me" for workspace app auth to avoid any security issues
		// caused by having an identical workspace name on yourself and a
		// different user and potentially reusing a ticket.
		//
		// This is also mitigated by storing the workspace/agent ID in the
		// ticket, but we block it here to be double safe.
		//
		// Subdomain apps have never been used with "me" from our code, and path
		// apps now have a redirect to remove the "me" from the URL.
		return xerrors.New(`username cannot be "me" in app requests`)
	}
	if r.WorkspaceAndAgent != "" {
		split := strings.Split(r.WorkspaceAndAgent, ".")
		if split[0] == "" || (len(split) == 2 && split[1] == "") || len(split) > 2 {
			return xerrors.Errorf("invalid workspace and agent: %q", r.WorkspaceAndAgent)
		}
		if r.WorkspaceNameOrID != "" || r.AgentNameOrID != "" {
			return xerrors.New("dev error: cannot specify both WorkspaceAndAgent and (WorkspaceNameOrID and AgentNameOrID)")
		}
	}
	if r.WorkspaceAndAgent == "" && r.WorkspaceNameOrID == "" {
		return xerrors.New("workspace name or ID is required")
	}
	if r.AppSlugOrPort == "" {
		return xerrors.New("app slug or port is required")
	}

	return nil
}
