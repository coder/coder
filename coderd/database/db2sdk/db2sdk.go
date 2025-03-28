// Package db2sdk provides common conversion routines from database types to codersdk types
package db2sdk

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
)

// List is a helper function to reduce boilerplate when converting slices of
// database types to slices of codersdk types.
// Only works if the function takes a single argument.
func List[F any, T any](list []F, convert func(F) T) []T {
	return ListLazy(convert)(list)
}

// ListLazy returns the converter function for a list, but does not eval
// the input. Helpful for combining the Map and the List functions.
func ListLazy[F any, T any](convert func(F) T) func(list []F) []T {
	return func(list []F) []T {
		into := make([]T, 0, len(list))
		for _, item := range list {
			into = append(into, convert(item))
		}
		return into
	}
}

func Map[K comparable, F any, T any](params map[K]F, convert func(F) T) map[K]T {
	into := make(map[K]T)
	for k, item := range params {
		into[k] = convert(item)
	}
	return into
}

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

func WorkspaceBuildParameter(p database.WorkspaceBuildParameter) codersdk.WorkspaceBuildParameter {
	return codersdk.WorkspaceBuildParameter{
		Name:  p.Name,
		Value: p.Value,
	}
}

func WorkspaceBuildParameters(params []database.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	return List(params, WorkspaceBuildParameter)
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

	descriptionPlaintext, err := render.PlaintextFromMarkdown(param.Description)
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

func ReducedUser(user database.User) codersdk.ReducedUser {
	return codersdk.ReducedUser{
		MinimalUser: codersdk.MinimalUser{
			ID:        user.ID,
			Username:  user.Username,
			AvatarURL: user.AvatarURL,
		},
		Email:      user.Email,
		Name:       user.Name,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
		LastSeenAt: user.LastSeenAt,
		Status:     codersdk.UserStatus(user.Status),
		LoginType:  codersdk.LoginType(user.LoginType),
	}
}

func UserFromGroupMember(member database.GroupMember) database.User {
	return database.User{
		ID:                 member.UserID,
		Email:              member.UserEmail,
		Username:           member.UserUsername,
		HashedPassword:     member.UserHashedPassword,
		CreatedAt:          member.UserCreatedAt,
		UpdatedAt:          member.UserUpdatedAt,
		Status:             member.UserStatus,
		RBACRoles:          member.UserRbacRoles,
		LoginType:          member.UserLoginType,
		AvatarURL:          member.UserAvatarUrl,
		Deleted:            member.UserDeleted,
		LastSeenAt:         member.UserLastSeenAt,
		QuietHoursSchedule: member.UserQuietHoursSchedule,
		Name:               member.UserName,
		GithubComUserID:    member.UserGithubComUserID,
	}
}

func ReducedUserFromGroupMember(member database.GroupMember) codersdk.ReducedUser {
	return ReducedUser(UserFromGroupMember(member))
}

func ReducedUsersFromGroupMembers(members []database.GroupMember) []codersdk.ReducedUser {
	return List(members, ReducedUserFromGroupMember)
}

func ReducedUsers(users []database.User) []codersdk.ReducedUser {
	return List(users, ReducedUser)
}

func User(user database.User, organizationIDs []uuid.UUID) codersdk.User {
	convertedUser := codersdk.User{
		ReducedUser:     ReducedUser(user),
		OrganizationIDs: organizationIDs,
		Roles:           SlimRolesFromNames(user.RBACRoles),
	}

	return convertedUser
}

func Users(users []database.User, organizationIDs map[uuid.UUID][]uuid.UUID) []codersdk.User {
	return List(users, func(user database.User) codersdk.User {
		return User(user, organizationIDs[user.ID])
	})
}

func Group(row database.GetGroupsRow, members []database.GroupMember, totalMemberCount int) codersdk.Group {
	return codersdk.Group{
		ID:                      row.Group.ID,
		Name:                    row.Group.Name,
		DisplayName:             row.Group.DisplayName,
		OrganizationID:          row.Group.OrganizationID,
		AvatarURL:               row.Group.AvatarURL,
		Members:                 ReducedUsersFromGroupMembers(members),
		TotalMemberCount:        totalMemberCount,
		QuotaAllowance:          int(row.Group.QuotaAllowance),
		Source:                  codersdk.GroupSource(row.Group.Source),
		OrganizationName:        row.OrganizationName,
		OrganizationDisplayName: row.OrganizationDisplayName,
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

			plaintextDescription, err := render.PlaintextFromMarkdown(param.Description)
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

func OAuth2ProviderApp(accessURL *url.URL, dbApp database.OAuth2ProviderApp) codersdk.OAuth2ProviderApp {
	return codersdk.OAuth2ProviderApp{
		ID:          dbApp.ID,
		Name:        dbApp.Name,
		CallbackURL: dbApp.CallbackURL,
		Icon:        dbApp.Icon,
		Endpoints: codersdk.OAuth2AppEndpoints{
			Authorization: accessURL.ResolveReference(&url.URL{
				Path: "/oauth2/authorize",
			}).String(),
			Token: accessURL.ResolveReference(&url.URL{
				Path: "/oauth2/tokens",
			}).String(),
			// We do not currently support DeviceAuth.
			DeviceAuth: "",
		},
	}
}

func OAuth2ProviderApps(accessURL *url.URL, dbApps []database.OAuth2ProviderApp) []codersdk.OAuth2ProviderApp {
	return List(dbApps, func(dbApp database.OAuth2ProviderApp) codersdk.OAuth2ProviderApp {
		return OAuth2ProviderApp(accessURL, dbApp)
	})
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

func WorkspaceAgentEnvironment(workspaceAgent database.WorkspaceAgent) (map[string]string, error) {
	var envs map[string]string
	if workspaceAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(workspaceAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal environment variables: %w", err)
		}
	}

	return envs, nil
}

func WorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator tailnet.Coordinator,
	dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, scripts []codersdk.WorkspaceAgentScript, logSources []codersdk.WorkspaceAgentLogSource,
	agentInactiveDisconnectTimeout time.Duration, agentFallbackTroubleshootingURL string,
) (codersdk.WorkspaceAgent, error) {
	envs, err := WorkspaceAgentEnvironment(dbAgent)
	if err != nil {
		return codersdk.WorkspaceAgent{}, err
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
	return appurl.ApplicationURL{
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

func Apps(dbApps []database.WorkspaceApp, statuses []database.WorkspaceAppStatus, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) []codersdk.WorkspaceApp {
	sort.Slice(dbApps, func(i, j int) bool {
		if dbApps[i].DisplayOrder != dbApps[j].DisplayOrder {
			return dbApps[i].DisplayOrder < dbApps[j].DisplayOrder
		}
		if dbApps[i].DisplayName != dbApps[j].DisplayName {
			return dbApps[i].DisplayName < dbApps[j].DisplayName
		}
		return dbApps[i].Slug < dbApps[j].Slug
	})

	statusesByAppID := map[uuid.UUID][]database.WorkspaceAppStatus{}
	for _, status := range statuses {
		statusesByAppID[status.AppID] = append(statusesByAppID[status.AppID], status)
	}

	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		statuses := statusesByAppID[dbApp.ID]
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
			Health:   codersdk.WorkspaceAppHealth(dbApp.Health),
			Hidden:   dbApp.Hidden,
			OpenIn:   codersdk.WorkspaceAppOpenIn(dbApp.OpenIn),
			Statuses: WorkspaceAppStatuses(statuses),
		})
	}
	return apps
}

func WorkspaceAppStatuses(statuses []database.WorkspaceAppStatus) []codersdk.WorkspaceAppStatus {
	return List(statuses, WorkspaceAppStatus)
}

func WorkspaceAppStatus(status database.WorkspaceAppStatus) codersdk.WorkspaceAppStatus {
	return codersdk.WorkspaceAppStatus{
		ID:                 status.ID,
		CreatedAt:          status.CreatedAt,
		AgentID:            status.AgentID,
		AppID:              status.AppID,
		NeedsUserAttention: status.NeedsUserAttention,
		URI:                status.Uri.String,
		Icon:               status.Icon.String,
		Message:            status.Message,
		State:              codersdk.WorkspaceAppStatusState(status.State),
	}
}

func ProvisionerDaemon(dbDaemon database.ProvisionerDaemon) codersdk.ProvisionerDaemon {
	result := codersdk.ProvisionerDaemon{
		ID:             dbDaemon.ID,
		OrganizationID: dbDaemon.OrganizationID,
		CreatedAt:      dbDaemon.CreatedAt,
		LastSeenAt:     codersdk.NullTime{NullTime: dbDaemon.LastSeenAt},
		Name:           dbDaemon.Name,
		Tags:           dbDaemon.Tags,
		Version:        dbDaemon.Version,
		APIVersion:     dbDaemon.APIVersion,
		KeyID:          dbDaemon.KeyID,
	}
	for _, provisionerType := range dbDaemon.Provisioners {
		result.Provisioners = append(result.Provisioners, codersdk.ProvisionerType(provisionerType))
	}
	return result
}

func RecentProvisionerDaemons(now time.Time, staleInterval time.Duration, daemons []database.ProvisionerDaemon) []codersdk.ProvisionerDaemon {
	results := []codersdk.ProvisionerDaemon{}

	for _, daemon := range daemons {
		// Daemon never connected, skip.
		if !daemon.LastSeenAt.Valid {
			continue
		}
		// Daemon has gone away, skip.
		if now.Sub(daemon.LastSeenAt.Time) > staleInterval {
			continue
		}

		results = append(results, ProvisionerDaemon(daemon))
	}

	// Ensure stable order for display and for tests
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}

func SlimRole(role rbac.Role) codersdk.SlimRole {
	orgID := ""
	if role.Identifier.OrganizationID != uuid.Nil {
		orgID = role.Identifier.OrganizationID.String()
	}

	return codersdk.SlimRole{
		DisplayName:    role.DisplayName,
		Name:           role.Identifier.Name,
		OrganizationID: orgID,
	}
}

func SlimRolesFromNames(names []string) []codersdk.SlimRole {
	convertedRoles := make([]codersdk.SlimRole, 0, len(names))

	for _, name := range names {
		convertedRoles = append(convertedRoles, SlimRoleFromName(name))
	}

	return convertedRoles
}

func SlimRoleFromName(name string) codersdk.SlimRole {
	rbacRole, err := rbac.RoleByName(rbac.RoleIdentifier{Name: name})
	var convertedRole codersdk.SlimRole
	if err == nil {
		convertedRole = SlimRole(rbacRole)
	} else {
		convertedRole = codersdk.SlimRole{Name: name}
	}
	return convertedRole
}

func RBACRole(role rbac.Role) codersdk.Role {
	slim := SlimRole(role)

	orgPerms := role.Org[slim.OrganizationID]
	return codersdk.Role{
		Name:                    slim.Name,
		OrganizationID:          slim.OrganizationID,
		DisplayName:             slim.DisplayName,
		SitePermissions:         List(role.Site, RBACPermission),
		OrganizationPermissions: List(orgPerms, RBACPermission),
		UserPermissions:         List(role.User, RBACPermission),
	}
}

func Role(role database.CustomRole) codersdk.Role {
	orgID := ""
	if role.OrganizationID.UUID != uuid.Nil {
		orgID = role.OrganizationID.UUID.String()
	}

	return codersdk.Role{
		Name:                    role.Name,
		OrganizationID:          orgID,
		DisplayName:             role.DisplayName,
		SitePermissions:         List(role.SitePermissions, Permission),
		OrganizationPermissions: List(role.OrgPermissions, Permission),
		UserPermissions:         List(role.UserPermissions, Permission),
	}
}

func Permission(permission database.CustomRolePermission) codersdk.Permission {
	return codersdk.Permission{
		Negate:       permission.Negate,
		ResourceType: codersdk.RBACResource(permission.ResourceType),
		Action:       codersdk.RBACAction(permission.Action),
	}
}

func RBACPermission(permission rbac.Permission) codersdk.Permission {
	return codersdk.Permission{
		Negate:       permission.Negate,
		ResourceType: codersdk.RBACResource(permission.ResourceType),
		Action:       codersdk.RBACAction(permission.Action),
	}
}

func Organization(organization database.Organization) codersdk.Organization {
	return codersdk.Organization{
		MinimalOrganization: codersdk.MinimalOrganization{
			ID:          organization.ID,
			Name:        organization.Name,
			DisplayName: organization.DisplayName,
			Icon:        organization.Icon,
		},
		Description: organization.Description,
		CreatedAt:   organization.CreatedAt,
		UpdatedAt:   organization.UpdatedAt,
		IsDefault:   organization.IsDefault,
	}
}

func CryptoKeys(keys []database.CryptoKey) []codersdk.CryptoKey {
	return List(keys, CryptoKey)
}

func CryptoKey(key database.CryptoKey) codersdk.CryptoKey {
	return codersdk.CryptoKey{
		Feature:   codersdk.CryptoKeyFeature(key.Feature),
		Sequence:  key.Sequence,
		StartsAt:  key.StartsAt,
		DeletesAt: key.DeletesAt.Time,
		Secret:    key.Secret.String,
	}
}

func MatchedProvisioners(provisionerDaemons []database.ProvisionerDaemon, now time.Time, staleInterval time.Duration) codersdk.MatchedProvisioners {
	minLastSeenAt := now.Add(-staleInterval)
	mostRecentlySeen := codersdk.NullTime{}
	var matched codersdk.MatchedProvisioners
	for _, provisioner := range provisionerDaemons {
		if !provisioner.LastSeenAt.Valid {
			continue
		}
		matched.Count++
		if provisioner.LastSeenAt.Time.After(minLastSeenAt) {
			matched.Available++
		}
		if provisioner.LastSeenAt.Time.After(mostRecentlySeen.Time) {
			matched.MostRecentlySeen.Valid = true
			matched.MostRecentlySeen.Time = provisioner.LastSeenAt.Time
		}
	}
	return matched
}

func TemplateRoleActions(role codersdk.TemplateRole) []policy.Action {
	switch role {
	case codersdk.TemplateRoleAdmin:
		return []policy.Action{policy.WildcardSymbol}
	case codersdk.TemplateRoleUse:
		return []policy.Action{policy.ActionRead, policy.ActionUse}
	}
	return []policy.Action{}
}

func AuditActionFromAgentProtoConnectionAction(action agentproto.Connection_Action) (database.AuditAction, error) {
	switch action {
	case agentproto.Connection_CONNECT:
		return database.AuditActionConnect, nil
	case agentproto.Connection_DISCONNECT:
		return database.AuditActionDisconnect, nil
	default:
		// Also Connection_ACTION_UNSPECIFIED, no mapping.
		return "", xerrors.Errorf("unknown agent connection action %q", action)
	}
}

func AgentProtoConnectionActionToAuditAction(action database.AuditAction) (agentproto.Connection_Action, error) {
	switch action {
	case database.AuditActionConnect:
		return agentproto.Connection_CONNECT, nil
	case database.AuditActionDisconnect:
		return agentproto.Connection_DISCONNECT, nil
	default:
		return agentproto.Connection_ACTION_UNSPECIFIED, xerrors.Errorf("unknown agent connection action %q", action)
	}
}
