package workspaceapps

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

type AccessMethod string

const (
	AccessMethodPath      AccessMethod = "path"
	AccessMethodSubdomain AccessMethod = "subdomain"
	// AccessMethodTerminal is special since it's not a real app and only
	// applies to the PTY endpoint on the API.
	AccessMethodTerminal AccessMethod = "terminal"
)

type Request struct {
	AccessMethod AccessMethod `json:"access_method"`
	// BasePath of the app. For path apps, this is the path prefix in the router
	// for this particular app. For subdomain apps, this should be "/". This is
	// used for setting the cookie path.
	BasePath string `json:"base_path"`

	// For the following fields, if the AccessMethod is AccessMethodTerminal,
	// then only AgentNameOrID may be set and it must be a UUID. The other
	// fields must be left blank.
	UsernameOrID string `json:"username_or_id"`
	// WorkspaceAndAgent xor WorkspaceNameOrID are required.
	WorkspaceAndAgent string `json:"-"` // "workspace" or "workspace.agent"
	WorkspaceNameOrID string `json:"workspace_name_or_id"`
	// AgentNameOrID is not required if the workspace has only one agent.
	AgentNameOrID string `json:"agent_name_or_id"`
	AppSlugOrPort string `json:"app_slug_or_port"`
}

func (r Request) Validate() error {
	switch r.AccessMethod {
	case AccessMethodPath, AccessMethodSubdomain, AccessMethodTerminal:
	default:
		return xerrors.Errorf("invalid access method: %q", r.AccessMethod)
	}
	if r.BasePath == "" {
		return xerrors.New("base path is required")
	}

	if r.AccessMethod == AccessMethodTerminal {
		if r.UsernameOrID != "" || r.WorkspaceAndAgent != "" || r.WorkspaceNameOrID != "" || r.AppSlugOrPort != "" {
			return xerrors.New("dev error: cannot specify any fields other than r.AccessMethod, r.BasePath and r.AgentNameOrID for terminal access method")
		}

		if r.AgentNameOrID == "" {
			return xerrors.New("agent name or ID is required")
		}
		if _, err := uuid.Parse(r.AgentNameOrID); err != nil {
			return xerrors.Errorf("invalid agent name or ID %q, must be a UUID: %w", r.AgentNameOrID, err)
		}

		return nil
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

type databaseRequest struct {
	Request
	// User is the user that owns the app.
	User database.User
	// Workspace is the workspace that the app is in.
	Workspace database.Workspace
	// Agent is the agent that the app is running on.
	Agent database.WorkspaceAgent

	// AppURL is the resolved URL to the workspace app. This is only set for non
	// terminal requests.
	AppURL string
	// AppHealth is the health of the app. For terminal requests, this is always
	// database.WorkspaceAppHealthHealthy.
	AppHealth database.WorkspaceAppHealth
	// AppSharingLevel is the sharing level of the app. This is forced to be set
	// to AppSharingLevelOwner if the access method is terminal.
	AppSharingLevel database.AppSharingLevel
}

// getDatabase does queries to get the owner user, workspace and agent
// associated with the app in the request. This will correctly perform the
// queries in the correct order based on the access method and what fields are
// available.
//
// If any of the queries don't return any rows, the error will wrap
// sql.ErrNoRows. All other errors should be considered internal server errors.
func (r Request) getDatabase(ctx context.Context, db database.Store) (*databaseRequest, error) {
	// If the AccessMethod is AccessMethodTerminal, then we need to get the
	// agent first since that's the only info we have.
	if r.AccessMethod == AccessMethodTerminal {
		return r.getDatabaseTerminal(ctx, db)
	}

	// For non-terminal requests, get the objects in order since we have all
	// fields available.

	// Get user.
	var (
		user    database.User
		userErr error
	)
	if userID, uuidErr := uuid.Parse(r.UsernameOrID); uuidErr == nil {
		user, userErr = db.GetUserByID(ctx, userID)
	} else {
		user, userErr = db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Username: r.UsernameOrID,
		})
	}
	if userErr != nil {
		return nil, xerrors.Errorf("get user %q: %w", r.UsernameOrID, userErr)
	}

	// Get workspace.
	var (
		workspace    database.Workspace
		workspaceErr error
	)
	if workspaceID, uuidErr := uuid.Parse(r.WorkspaceNameOrID); uuidErr == nil {
		workspace, workspaceErr = db.GetWorkspaceByID(ctx, workspaceID)
	} else {
		workspace, workspaceErr = db.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: user.ID,
			Name:    r.WorkspaceNameOrID,
			Deleted: false,
		})
	}
	if workspaceErr != nil {
		return nil, xerrors.Errorf("get workspace %q: %w", r.WorkspaceNameOrID, workspaceErr)
	}

	// Get workspace agents.
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	if len(agents) == 0 {
		// TODO(@deansheather): return a 404 if there are no agents in the
		// workspace, requires a different error type.
		return nil, xerrors.New("no agents in workspace")
	}

	// Get workspace apps.
	agentIDs := make([]uuid.UUID, len(agents))
	for i, agent := range agents {
		agentIDs[i] = agent.ID
	}
	apps, err := db.GetWorkspaceAppsByAgentIDs(ctx, agentIDs)
	if err != nil {
		return nil, xerrors.Errorf("get workspace apps: %w", err)
	}

	// Get the app first, because r.AgentNameOrID is optional depending on
	// whether the app is a slug or a port and whether there are multiple agents
	// in the workspace or not.
	var (
		agentNameOrID         = r.AgentNameOrID
		appURL                string
		appSharingLevel       database.AppSharingLevel
		appHealth             = database.WorkspaceAppHealthDisabled
		portUint, portUintErr = strconv.ParseUint(r.AppSlugOrPort, 10, 16)
	)
	if portUintErr == nil {
		if r.AccessMethod != AccessMethodSubdomain {
			// TODO(@deansheather): this should return a 400 instead of a 500.
			return nil, xerrors.New("port-based URLs are only supported for subdomain-based applications")
		}

		// If the user specified a port, then they must specify the agent if
		// there are multiple agents in the workspace. App names are unique per
		// workspace.
		if agentNameOrID == "" {
			if len(agents) != 1 {
				return nil, xerrors.New("port specified with no agent, but multiple agents exist in the workspace")
			}
			agentNameOrID = agents[0].ID.String()
		}

		// If the app slug is a port number, then route to the port as an
		// "anonymous app". We only support HTTP for port-based URLs.
		//
		// This is only supported for subdomain-based applications.
		appURL = fmt.Sprintf("http://127.0.0.1:%d", portUint)
		appSharingLevel = database.AppSharingLevelOwner
	} else {
		for _, app := range apps {
			if app.Slug == r.AppSlugOrPort {
				if !app.Url.Valid {
					return nil, xerrors.Errorf("app URL is not valid")
				}

				agentNameOrID = app.AgentID.String()
				if app.SharingLevel != "" {
					appSharingLevel = app.SharingLevel
				} else {
					appSharingLevel = database.AppSharingLevelOwner
				}
				appURL = app.Url.String
				appHealth = app.Health
				break
			}
		}
	}
	if appURL == "" {
		return nil, xerrors.Errorf("no app found with slug %q: %w", r.AppSlugOrPort, sql.ErrNoRows)
	}

	// Finally, get agent.
	var agent database.WorkspaceAgent
	if agentID, uuidErr := uuid.Parse(agentNameOrID); uuidErr == nil {
		for _, a := range agents {
			if a.ID == agentID {
				agent = a
				break
			}
		}
	} else {
		if agentNameOrID == "" && len(agents) == 1 {
			agent = agents[0]
		} else {
			for _, a := range agents {
				if a.Name == agentNameOrID {
					agent = a
					break
				}
			}
		}

		if agent.ID == uuid.Nil {
			return nil, xerrors.Errorf("no agent found with name %q: %w", r.AgentNameOrID, sql.ErrNoRows)
		}
	}

	return &databaseRequest{
		Request:         r,
		User:            user,
		Workspace:       workspace,
		Agent:           agent,
		AppURL:          appURL,
		AppHealth:       appHealth,
		AppSharingLevel: appSharingLevel,
	}, nil
}

// getDatabaseTerminal is called by getDatabase for AccessMethodTerminal
// requests.
func (r Request) getDatabaseTerminal(ctx context.Context, db database.Store) (*databaseRequest, error) {
	if r.AccessMethod != AccessMethodTerminal {
		return nil, xerrors.Errorf("invalid access method %q for terminal request", r.AccessMethod)
	}

	agentID, uuidErr := uuid.Parse(r.AgentNameOrID)
	if uuidErr != nil {
		return nil, xerrors.Errorf("invalid agent name or ID %q, must be a UUID for terminal requests: %w", r.AgentNameOrID, uuidErr)
	}

	var err error
	agent, err := db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agent %q: %w", agentID, err)
	}

	// Get the corresponding resource.
	res, err := db.GetWorkspaceResourceByID(ctx, agent.ResourceID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agent resource %q: %w", agent.ResourceID, err)
	}

	// Get the corresponding workspace build.
	build, err := db.GetWorkspaceBuildByJobID(ctx, res.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace build by job ID %q: %w", res.JobID, err)
	}

	// Get the corresponding workspace.
	workspace, err := db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace %q: %w", build.WorkspaceID, err)
	}

	// Get the workspace's owner.
	user, err := db.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		return nil, xerrors.Errorf("get user %q: %w", workspace.OwnerID, err)
	}

	return &databaseRequest{
		Request:         r,
		User:            user,
		Workspace:       workspace,
		Agent:           agent,
		AppURL:          "",
		AppHealth:       database.WorkspaceAppHealthHealthy,
		AppSharingLevel: database.AppSharingLevelOwner,
	}, nil
}
