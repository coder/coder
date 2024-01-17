// Package db2sdk provides common conversion routines from database types to codersdk types
package db2sdk

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/parameter"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
)

type ExternalAuthMeta struct {
	Authenticated bool
	ValidateError string
}

func ExternalAuths(auths []database.ExternalAuthLink, meta map[string]ExternalAuthMeta) []codersdk.ExternalAuthLink {
	out := make([]codersdk.ExternalAuthLink, 0, len(auths))
	for _, auth := range auths {
		out = append(out, ExternalAuth(auth, meta[auth.ProviderID]))
	}
	return out
}

func ExternalAuth(auth database.ExternalAuthLink, meta ExternalAuthMeta) codersdk.ExternalAuthLink {
	return codersdk.ExternalAuthLink{
		ProviderID:      auth.ProviderID,
		CreatedAt:       auth.CreatedAt,
		UpdatedAt:       auth.UpdatedAt,
		HasRefreshToken: auth.OAuthRefreshToken != "",
		Expires:         auth.OAuthExpiry,
		Authenticated:   meta.Authenticated,
		ValidateError:   meta.ValidateError,
	}
}

func WorkspaceBuildParameters(params []database.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	out := make([]codersdk.WorkspaceBuildParameter, len(params))
	for i, p := range params {
		out[i] = WorkspaceBuildParameter(p)
	}
	return out
}

func WorkspaceBuildParameter(p database.WorkspaceBuildParameter) codersdk.WorkspaceBuildParameter {
	return codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: p.Value,
	}
}

func TemplateVersionParameters(params []database.TemplateVersionParameter) ([]codersdk.TemplateVersionParameter, error) {
	out := make([]codersdk.TemplateVersionParameter, len(params))
	var err error
	for i, p := range params {
		out[i], err = TemplateVersionParameter(p)
		if err != nil {
			return nil, xerrors.Errorf("convert template version parameter %q: %w", p.Name, err)
		}
	}

	return out, nil
}

func TemplateVersionParameter(param database.TemplateVersionParameter) (codersdk.TemplateVersionParameter, error) {
	options, err := templateVersionParameterOptions(param.Options)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}

	descriptionPlaintext, err := parameter.Plaintext(param.Description)
	if err != nil {
		return codersdk.TemplateVersionParameter{}, err
	}

	var validationMin *int32
	if param.ValidationMin.Valid {
		validationMin = &param.ValidationMin.Int32
	}

	var validationMax *int32
	if param.ValidationMax.Valid {
		validationMax = &param.ValidationMax.Int32
	}

	return codersdk.TemplateVersionParameter{
		Name:                 param.Name,
		DisplayName:          param.DisplayName,
		Description:          param.Description,
		DescriptionPlaintext: descriptionPlaintext,
		Type:                 param.Type,
		Mutable:              param.Mutable,
		DefaultValue:         param.DefaultValue,
		Icon:                 param.Icon,
		Options:              options,
		ValidationRegex:      param.ValidationRegex,
		ValidationMin:        validationMin,
		ValidationMax:        validationMax,
		ValidationError:      param.ValidationError,
		ValidationMonotonic:  codersdk.ValidationMonotonicOrder(param.ValidationMonotonic),
		Required:             param.Required,
		Ephemeral:            param.Ephemeral,
	}, nil
}

func User(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	convertedUser := codersdk.User{
		ID:              user.ID,
		Email:           user.Email,
		Name:            user.Name,
		CreatedAt:       user.CreatedAt,
		LastSeenAt:      user.LastSeenAt,
		Username:        user.Username,
		Status:          codersdk.UserStatus(user.Status),
		OrganizationIDs: organizationIDs,
		Roles:           make([]codersdk.Role, 0, len(user.RBACRoles)),
		AvatarURL:       user.AvatarURL,
		LoginType:       codersdk.LoginType(user.LoginType),
		ThemePreference: user.ThemePreference,
	}

	for _, roleName := range user.RBACRoles {
		rbacRole, _ := rbac.RoleByName(roleName)
		convertedUser.Roles = append(convertedUser.Roles, Role(rbacRole))
	}

	return convertedUser
}

func Role(role rbac.Role) codersdk.Role {
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

func TemplateInsightsParameters(parameterRows []database.GetTemplateParameterInsightsRow) ([]codersdk.TemplateParameterUsage, error) {
	// Use a stable sort, similarly to how we would sort in the query, note that
	// we don't sort in the query because order varies depending on the table
	// collation.
	//
	// ORDER BY utp.name, utp.type, utp.display_name, utp.description, utp.options, wbp.value
	slices.SortFunc(parameterRows, func(a, b database.GetTemplateParameterInsightsRow) int {
		if a.Name != b.Name {
			return strings.Compare(a.Name, b.Name)
		}
		if a.Type != b.Type {
			return strings.Compare(a.Type, b.Type)
		}
		if a.DisplayName != b.DisplayName {
			return strings.Compare(a.DisplayName, b.DisplayName)
		}
		if a.Description != b.Description {
			return strings.Compare(a.Description, b.Description)
		}
		if string(a.Options) != string(b.Options) {
			return strings.Compare(string(a.Options), string(b.Options))
		}
		return strings.Compare(a.Value, b.Value)
	})

	parametersUsage := []codersdk.TemplateParameterUsage{}
	indexByNum := make(map[int64]int)
	for _, param := range parameterRows {
		if _, ok := indexByNum[param.Num]; !ok {
			var opts []codersdk.TemplateVersionParameterOption
			err := json.Unmarshal(param.Options, &opts)
			if err != nil {
				return nil, err
			}

			plaintextDescription, err := parameter.Plaintext(param.Description)
			if err != nil {
				return nil, err
			}

			parametersUsage = append(parametersUsage, codersdk.TemplateParameterUsage{
				TemplateIDs: param.TemplateIDs,
				Name:        param.Name,
				Type:        param.Type,
				DisplayName: param.DisplayName,
				Description: plaintextDescription,
				Options:     opts,
			})
			indexByNum[param.Num] = len(parametersUsage) - 1
		}

		i := indexByNum[param.Num]
		parametersUsage[i].Values = append(parametersUsage[i].Values, codersdk.TemplateParameterValue{
			Value: param.Value,
			Count: param.Count,
		})
	}

	return parametersUsage, nil
}

func templateVersionParameterOptions(rawOptions json.RawMessage) ([]codersdk.TemplateVersionParameterOption, error) {
	var protoOptions []*proto.RichParameterOption
	err := json.Unmarshal(rawOptions, &protoOptions)
	if err != nil {
		return nil, err
	}
	var options []codersdk.TemplateVersionParameterOption
	for _, option := range protoOptions {
		options = append(options, codersdk.TemplateVersionParameterOption{
			Name:        option.Name,
			Description: option.Description,
			Value:       option.Value,
			Icon:        option.Icon,
		})
	}
	return options, nil
}

func OAuth2ProviderApp(dbApp database.OAuth2ProviderApp) codersdk.OAuth2ProviderApp {
	return codersdk.OAuth2ProviderApp{
		ID:          dbApp.ID,
		Name:        dbApp.Name,
		CallbackURL: dbApp.CallbackURL,
		Icon:        dbApp.Icon,
	}
}

func OAuth2ProviderApps(dbApps []database.OAuth2ProviderApp) []codersdk.OAuth2ProviderApp {
	apps := []codersdk.OAuth2ProviderApp{}
	for _, dbApp := range dbApps {
		apps = append(apps, OAuth2ProviderApp(dbApp))
	}
	return apps
}

func convertDisplayApps(apps []database.DisplayApp) []codersdk.DisplayApp {
	dapps := make([]codersdk.DisplayApp, 0, len(apps))
	for _, app := range apps {
		switch codersdk.DisplayApp(app) {
		case codersdk.DisplayAppVSCodeDesktop, codersdk.DisplayAppVSCodeInsiders, codersdk.DisplayAppPortForward, codersdk.DisplayAppWebTerminal, codersdk.DisplayAppSSH:
			dapps = append(dapps, codersdk.DisplayApp(app))
		}
	}

	return dapps
}

func WorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator tailnet.Coordinator,
	dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, scripts []codersdk.WorkspaceAgentScript, logSources []codersdk.WorkspaceAgentLogSource,
	agentInactiveDisconnectTimeout time.Duration, agentFallbackTroubleshootingURL string,
) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal env vars: %w", err)
		}
	}
	troubleshootingURL := agentFallbackTroubleshootingURL
	if dbAgent.TroubleshootingURL != "" {
		troubleshootingURL = dbAgent.TroubleshootingURL
	}
	subsystems := make([]codersdk.AgentSubsystem, len(dbAgent.Subsystems))
	for i, subsystem := range dbAgent.Subsystems {
		subsystems[i] = codersdk.AgentSubsystem(subsystem)
	}

	legacyStartupScriptBehavior := codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking
	for _, script := range scripts {
		if !script.RunOnStart {
			continue
		}
		if !script.StartBlocksLogin {
			continue
		}
		legacyStartupScriptBehavior = codersdk.WorkspaceAgentStartupScriptBehaviorBlocking
	}

	workspaceAgent := codersdk.WorkspaceAgent{
		ID:                       dbAgent.ID,
		CreatedAt:                dbAgent.CreatedAt,
		UpdatedAt:                dbAgent.UpdatedAt,
		ResourceID:               dbAgent.ResourceID,
		InstanceID:               dbAgent.AuthInstanceID.String,
		Name:                     dbAgent.Name,
		Architecture:             dbAgent.Architecture,
		OperatingSystem:          dbAgent.OperatingSystem,
		Scripts:                  scripts,
		StartupScriptBehavior:    legacyStartupScriptBehavior,
		LogsLength:               dbAgent.LogsLength,
		LogsOverflowed:           dbAgent.LogsOverflowed,
		LogSources:               logSources,
		Version:                  dbAgent.Version,
		APIVersion:               dbAgent.APIVersion,
		EnvironmentVariables:     envs,
		Directory:                dbAgent.Directory,
		ExpandedDirectory:        dbAgent.ExpandedDirectory,
		Apps:                     apps,
		ConnectionTimeoutSeconds: dbAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       troubleshootingURL,
		LifecycleState:           codersdk.WorkspaceAgentLifecycle(dbAgent.LifecycleState),
		Subsystems:               subsystems,
		DisplayApps:              convertDisplayApps(dbAgent.DisplayApps),
	}
	node := coordinator.Node(dbAgent.ID)
	if node != nil {
		workspaceAgent.DERPLatency = map[string]codersdk.DERPRegion{}
		for rawRegion, latency := range node.DERPLatency {
			regionParts := strings.SplitN(rawRegion, "-", 2)
			regionID, err := strconv.Atoi(regionParts[0])
			if err != nil {
				return codersdk.WorkspaceAgent{}, xerrors.Errorf("convert derp region id %q: %w", rawRegion, err)
			}
			region, found := derpMap.Regions[regionID]
			if !found {
				// It's possible that a workspace agent is using an old DERPMap
				// and reports regions that do not exist. If that's the case,
				// report the region as unknown!
				region = &tailcfg.DERPRegion{
					RegionID:   regionID,
					RegionName: fmt.Sprintf("Unnamed %d", regionID),
				}
			}
			workspaceAgent.DERPLatency[region.RegionName] = codersdk.DERPRegion{
				Preferred:           node.PreferredDERP == regionID,
				LatencyMilliseconds: latency * 1000,
			}
		}
	}

	status := dbAgent.Status(agentInactiveDisconnectTimeout)
	workspaceAgent.Status = codersdk.WorkspaceAgentStatus(status.Status)
	workspaceAgent.FirstConnectedAt = status.FirstConnectedAt
	workspaceAgent.LastConnectedAt = status.LastConnectedAt
	workspaceAgent.DisconnectedAt = status.DisconnectedAt

	if dbAgent.StartedAt.Valid {
		workspaceAgent.StartedAt = &dbAgent.StartedAt.Time
	}
	if dbAgent.ReadyAt.Valid {
		workspaceAgent.ReadyAt = &dbAgent.ReadyAt.Time
	}

	switch {
	case workspaceAgent.Status != codersdk.WorkspaceAgentConnected && workspaceAgent.LifecycleState == codersdk.WorkspaceAgentLifecycleOff:
		workspaceAgent.Health.Reason = "agent is not running"
	case workspaceAgent.Status == codersdk.WorkspaceAgentTimeout:
		workspaceAgent.Health.Reason = "agent is taking too long to connect"
	case workspaceAgent.Status == codersdk.WorkspaceAgentDisconnected:
		workspaceAgent.Health.Reason = "agent has lost connection"
	// Note: We could also handle codersdk.WorkspaceAgentLifecycleStartTimeout
	// here, but it's more of a soft issue, so we don't want to mark the agent
	// as unhealthy.
	case workspaceAgent.LifecycleState == codersdk.WorkspaceAgentLifecycleStartError:
		workspaceAgent.Health.Reason = "agent startup script exited with an error"
	case workspaceAgent.LifecycleState.ShuttingDown():
		workspaceAgent.Health.Reason = "agent is shutting down"
	default:
		workspaceAgent.Health.Healthy = true
	}

	return workspaceAgent, nil
}

func AppSubdomain(dbApp database.WorkspaceApp, agentName, workspaceName, ownerName string) string {
	if !dbApp.Subdomain || agentName == "" || ownerName == "" || workspaceName == "" {
		return ""
	}

	appSlug := dbApp.Slug
	if appSlug == "" {
		appSlug = dbApp.DisplayName
	}
	return httpapi.ApplicationURL{
		// We never generate URLs with a prefix. We only allow prefixes when
		// parsing URLs from the hostname. Users that want this feature can
		// write out their own URLs.
		Prefix:        "",
		AppSlugOrPort: appSlug,
		AgentName:     agentName,
		WorkspaceName: workspaceName,
		Username:      ownerName,
	}.String()
}

func Apps(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) []codersdk.WorkspaceApp {
	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		apps = append(apps, codersdk.WorkspaceApp{
			ID:            dbApp.ID,
			URL:           dbApp.Url.String,
			External:      dbApp.External,
			Slug:          dbApp.Slug,
			DisplayName:   dbApp.DisplayName,
			Command:       dbApp.Command.String,
			Icon:          dbApp.Icon,
			Subdomain:     dbApp.Subdomain,
			SubdomainName: AppSubdomain(dbApp, agent.Name, workspace.Name, ownerName),
			SharingLevel:  codersdk.WorkspaceAppSharingLevel(dbApp.SharingLevel),
			Healthcheck: codersdk.Healthcheck{
				URL:       dbApp.HealthcheckUrl,
				Interval:  dbApp.HealthcheckInterval,
				Threshold: dbApp.HealthcheckThreshold,
			},
			Health: codersdk.WorkspaceAppHealth(dbApp.Health),
		})
	}
	return apps
}

func ProvisionerDaemon(dbDaemon database.ProvisionerDaemon) codersdk.ProvisionerDaemon {
	result := codersdk.ProvisionerDaemon{
		ID:         dbDaemon.ID,
		CreatedAt:  dbDaemon.CreatedAt,
		LastSeenAt: codersdk.NullTime{NullTime: dbDaemon.LastSeenAt},
		Name:       dbDaemon.Name,
		Tags:       dbDaemon.Tags,
		Version:    dbDaemon.Version,
		APIVersion: dbDaemon.APIVersion,
	}
	for _, provisionerType := range dbDaemon.Provisioners {
		result.Provisioners = append(result.Provisioners, codersdk.ProvisionerType(provisionerType))
	}
	return result
}
