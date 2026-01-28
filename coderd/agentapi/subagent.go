package agentapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/quartz"
)

type SubAgentAPI struct {
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	AgentID        uuid.UUID
	AgentFn        func(context.Context) (database.WorkspaceAgent, error)

	Log      slog.Logger
	Clock    quartz.Clock
	Database database.Store
}

func (a *SubAgentAPI) CreateSubAgent(ctx context.Context, req *agentproto.CreateSubAgentRequest) (*agentproto.CreateSubAgentResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	createdAt := a.Clock.Now()

	displayApps := make([]database.DisplayApp, 0, len(req.DisplayApps))
	for idx, displayApp := range req.DisplayApps {
		var app database.DisplayApp

		switch displayApp {
		case agentproto.CreateSubAgentRequest_PORT_FORWARDING_HELPER:
			app = database.DisplayAppPortForwardingHelper
		case agentproto.CreateSubAgentRequest_SSH_HELPER:
			app = database.DisplayAppSSHHelper
		case agentproto.CreateSubAgentRequest_VSCODE:
			app = database.DisplayAppVscode
		case agentproto.CreateSubAgentRequest_VSCODE_INSIDERS:
			app = database.DisplayAppVscodeInsiders
		case agentproto.CreateSubAgentRequest_WEB_TERMINAL:
			app = database.DisplayAppWebTerminal
		default:
			return nil, codersdk.ValidationError{
				Field:  fmt.Sprintf("display_apps[%d]", idx),
				Detail: fmt.Sprintf("%q is not a valid display app", displayApp),
			}
		}

		displayApps = append(displayApps, app)
	}

	parentAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get parent agent: %w", err)
	}

	// An ID is only given in the request when it is a terraform-defined devcontainer
	// that has attached resources. These subagents are pre-provisioned by terraform
	// (the agent record already exists), so we update configurable fields like
	// display_apps rather than creating a new agent.
	if req.Id != nil {
		id, err := uuid.FromBytes(req.Id)
		if err != nil {
			return nil, xerrors.Errorf("parse id: %w", err)
		}

		subAgent, err := a.Database.GetWorkspaceAgentByID(ctx, id)
		if err != nil {
			return nil, xerrors.Errorf("get workspace agent by id: %w", err)
		}

		// Validate that the subagent belongs to the current parent agent to
		// prevent updating subagents from other agents within the same workspace.
		if !subAgent.ParentID.Valid || subAgent.ParentID.UUID != parentAgent.ID {
			return nil, xerrors.Errorf("subagent does not belong to this parent agent")
		}

		if err := a.Database.UpdateWorkspaceAgentDisplayAppsByID(ctx, database.UpdateWorkspaceAgentDisplayAppsByIDParams{
			ID:          id,
			DisplayApps: displayApps,
			UpdatedAt:   createdAt,
		}); err != nil {
			return nil, xerrors.Errorf("update workspace agent display apps: %w", err)
		}

		return &agentproto.CreateSubAgentResponse{
			Agent: &agentproto.SubAgent{
				Name:      subAgent.Name,
				Id:        subAgent.ID[:],
				AuthToken: subAgent.AuthToken[:],
			},
		}, nil
	}

	agentName := req.Name
	if agentName == "" {
		return nil, codersdk.ValidationError{
			Field:  "name",
			Detail: "agent name cannot be empty",
		}
	}
	if !provisioner.AgentNameRegex.MatchString(agentName) {
		return nil, codersdk.ValidationError{
			Field:  "name",
			Detail: fmt.Sprintf("agent name %q does not match regex %q", agentName, provisioner.AgentNameRegex),
		}
	}
	subAgent, err := a.Database.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
		ID:                       uuid.New(),
		ParentID:                 uuid.NullUUID{Valid: true, UUID: parentAgent.ID},
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
		Name:                     agentName,
		ResourceID:               parentAgent.ResourceID,
		AuthToken:                uuid.New(),
		AuthInstanceID:           parentAgent.AuthInstanceID,
		Architecture:             req.Architecture,
		EnvironmentVariables:     pqtype.NullRawMessage{},
		OperatingSystem:          req.OperatingSystem,
		Directory:                req.Directory,
		InstanceMetadata:         pqtype.NullRawMessage{},
		ResourceMetadata:         pqtype.NullRawMessage{},
		ConnectionTimeoutSeconds: parentAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       parentAgent.TroubleshootingURL,
		MOTDFile:                 "",
		DisplayApps:              displayApps,
		DisplayOrder:             0,
		APIKeyScope:              parentAgent.APIKeyScope,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert sub agent: %w", err)
	}

	var appCreationErrors []*agentproto.CreateSubAgentResponse_AppCreationError
	appSlugs := make(map[string]struct{})

	for i, app := range req.Apps {
		err := func() error {
			slug := app.Slug
			if slug == "" {
				return codersdk.ValidationError{
					Field:  "slug",
					Detail: "must not be empty",
				}
			}
			if !provisioner.AppSlugRegex.MatchString(slug) {
				return codersdk.ValidationError{
					Field:  "slug",
					Detail: fmt.Sprintf("%q does not match regex %q", slug, provisioner.AppSlugRegex),
				}
			}
			if _, exists := appSlugs[slug]; exists {
				return codersdk.ValidationError{
					Field:  "slug",
					Detail: fmt.Sprintf("%q is already in use", slug),
				}
			}
			appSlugs[slug] = struct{}{}

			health := database.WorkspaceAppHealthDisabled
			if app.Healthcheck == nil {
				app.Healthcheck = &agentproto.CreateSubAgentRequest_App_Healthcheck{}
			}
			if app.Healthcheck.Url != "" {
				health = database.WorkspaceAppHealthInitializing
			}

			share := app.GetShare()
			protoSharingLevel, ok := agentproto.CreateSubAgentRequest_App_SharingLevel_name[int32(share)]
			if !ok {
				return codersdk.ValidationError{
					Field:  "share",
					Detail: fmt.Sprintf("%q is not a valid app sharing level", share.String()),
				}
			}
			sharingLevel := database.AppSharingLevel(strings.ToLower(protoSharingLevel))

			var openIn database.WorkspaceAppOpenIn
			switch app.GetOpenIn() {
			case agentproto.CreateSubAgentRequest_App_SLIM_WINDOW:
				openIn = database.WorkspaceAppOpenInSlimWindow
			case agentproto.CreateSubAgentRequest_App_TAB:
				openIn = database.WorkspaceAppOpenInTab
			default:
				return codersdk.ValidationError{
					Field:  "open_in",
					Detail: fmt.Sprintf("%q is not an open in setting", app.GetOpenIn()),
				}
			}

			// NOTE(DanielleMaywood):
			// Slugs must be unique PER workspace/template. As of 2025-06-25,
			// there is no database-layer enforcement of this constraint.
			// We can get around this by creating a slug that *should* be
			// unique (at least highly probable).
			slugHash := sha256.Sum256([]byte(subAgent.Name + "/" + app.Slug))
			slugHashEnc := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(slugHash[:])
			computedSlug := strings.ToLower(slugHashEnc[:8]) + "-" + app.Slug

			_, err := a.Database.UpsertWorkspaceApp(ctx, database.UpsertWorkspaceAppParams{
				ID:          uuid.New(), // NOTE: we may need to maintain the app's ID here for stability, but for now we'll leave this as-is.
				CreatedAt:   createdAt,
				AgentID:     subAgent.ID,
				Slug:        computedSlug,
				DisplayName: app.GetDisplayName(),
				Icon:        app.GetIcon(),
				Command: sql.NullString{
					Valid:  app.GetCommand() != "",
					String: app.GetCommand(),
				},
				Url: sql.NullString{
					Valid:  app.GetUrl() != "",
					String: app.GetUrl(),
				},
				External:             app.GetExternal(),
				Subdomain:            app.GetSubdomain(),
				SharingLevel:         sharingLevel,
				HealthcheckUrl:       app.Healthcheck.Url,
				HealthcheckInterval:  app.Healthcheck.Interval,
				HealthcheckThreshold: app.Healthcheck.Threshold,
				Health:               health,
				DisplayOrder:         app.GetOrder(),
				Hidden:               app.GetHidden(),
				OpenIn:               openIn,
				DisplayGroup: sql.NullString{
					Valid:  app.GetGroup() != "",
					String: app.GetGroup(),
				},
				Tooltip: "", // tooltips are not currently supported in subagent workspaces, default to empty string
			})
			if err != nil {
				return xerrors.Errorf("insert workspace app: %w", err)
			}

			return nil
		}()
		if err != nil {
			appErr := &agentproto.CreateSubAgentResponse_AppCreationError{
				Index: int32(i), //nolint:gosec // This would only overflow if we created 2 billion apps.
				Error: err.Error(),
			}

			var validationErr codersdk.ValidationError
			if errors.As(err, &validationErr) {
				appErr.Field = &validationErr.Field
				appErr.Error = validationErr.Detail
			}

			appCreationErrors = append(appCreationErrors, appErr)
		}
	}

	return &agentproto.CreateSubAgentResponse{
		Agent: &agentproto.SubAgent{
			Name:      subAgent.Name,
			Id:        subAgent.ID[:],
			AuthToken: subAgent.AuthToken[:],
		},
		AppCreationErrors: appCreationErrors,
	}, nil
}

func (a *SubAgentAPI) DeleteSubAgent(ctx context.Context, req *agentproto.DeleteSubAgentRequest) (*agentproto.DeleteSubAgentResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	subAgentID, err := uuid.FromBytes(req.Id)
	if err != nil {
		return nil, err
	}

	if err := a.Database.DeleteWorkspaceSubAgentByID(ctx, subAgentID); err != nil {
		return nil, err
	}

	return &agentproto.DeleteSubAgentResponse{}, nil
}

func (a *SubAgentAPI) ListSubAgents(ctx context.Context, _ *agentproto.ListSubAgentsRequest) (*agentproto.ListSubAgentsResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	workspaceAgents, err := a.Database.GetWorkspaceAgentsByParentID(ctx, a.AgentID)
	if err != nil {
		return nil, err
	}

	agents := make([]*agentproto.SubAgent, len(workspaceAgents))

	for i, agent := range workspaceAgents {
		agents[i] = &agentproto.SubAgent{
			Name:      agent.Name,
			Id:        agent.ID[:],
			AuthToken: agent.AuthToken[:],
		}
	}

	return &agentproto.ListSubAgentsResponse{Agents: agents}, nil
}
