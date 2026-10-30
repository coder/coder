package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

// Every agent acts as its own Coder user: an agent user that, like a
// service account, has no email and cannot sign in. Chats in the agent are owned by that user, so its workspaces,
// audit entries, group memberships, and external auth links are the
// agent's rather than whoever started the chat.

const chatProjectAgentUsernamePrefix = "agent-"

var chatProjectAgentUsernameInvalid = regexp.MustCompile(`[^a-z0-9]+`)

// chatProjectAgentUsername derives a valid username from an agent name.
// suffix, when set, disambiguates agents with the same name.
func chatProjectAgentUsername(name, suffix string) string {
	slug := strings.Trim(chatProjectAgentUsernameInvalid.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if suffix != "" {
		suffix = "-" + suffix
	}
	maxBase := 32 - len(suffix)
	base := chatProjectAgentUsernamePrefix + slug
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	username := strings.TrimRight(base, "-") + suffix
	if codersdk.NameValid(username) != nil {
		return ""
	}
	return username
}

// createChatProjectAgentUser creates the agent user an agent acts as.
// store may be a transaction. Agent users are created by the system, so
// they do not need the Premium service accounts feature, and they do not
// count toward the license seat limit.
func (api *API) createChatProjectAgentUser(ctx context.Context, store database.Store, organizationID uuid.UUID, name, icon string) (database.User, error) {
	//nolint:gocritic // Agent users are system-managed.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	var username string
	for attempt := 0; attempt < 5 && username == ""; attempt++ {
		suffix := ""
		if attempt > 0 {
			random, err := cryptorand.StringCharset(cryptorand.Lower+cryptorand.Numeric, 5)
			if err != nil {
				return database.User{}, xerrors.Errorf("generate username suffix: %w", err)
			}
			suffix = random
		}
		candidate := chatProjectAgentUsername(name, suffix)
		if candidate == "" {
			continue
		}
		_, err := store.GetUserByEmailOrUsername(sysCtx, database.GetUserByEmailOrUsernameParams{Username: candidate})
		if errors.Is(err, sql.ErrNoRows) {
			username = candidate
			continue
		}
		if err != nil {
			return database.User{}, xerrors.Errorf("check agent username: %w", err)
		}
	}
	if username == "" {
		return database.User{}, xerrors.New("no available username for agent")
	}

	user, err := api.CreateUser(sysCtx, store, CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Username:        username,
			Name:            name,
			UserLoginType:   codersdk.LoginTypeNone,
			OrganizationIDs: []uuid.UUID{organizationID},
			// Agent users never sign in, so they would otherwise stay
			// dormant and could not run chats.
			UserStatus: ptr.Ref(codersdk.UserStatusActive),
		},
		LoginType:         database.LoginTypeNone,
		IsAgent:           true,
		SkipNotifications: true,
	})
	if err != nil {
		return database.User{}, xerrors.Errorf("create agent user: %w", err)
	}
	// The organization's default roles may not include chat access, so
	// grant the role that lets members use Coder Agents explicitly.
	if _, err := store.UpdateMemberRoles(sysCtx, database.UpdateMemberRolesParams{
		GrantedRoles: []string{codersdk.RoleAgentsAccess},
		UserID:       user.ID,
		OrgID:        organizationID,
	}); err != nil {
		return database.User{}, xerrors.Errorf("grant agent user chat access: %w", err)
	}
	if icon != "" {
		user, err = store.UpdateUserProfile(sysCtx, database.UpdateUserProfileParams{
			ID:        user.ID,
			Email:     user.Email,
			Username:  user.Username,
			AvatarURL: icon,
			UpdatedAt: dbtime.Now(),
			Name:      user.Name,
		})
		if err != nil {
			return database.User{}, xerrors.Errorf("set agent user avatar: %w", err)
		}
	}
	return user, nil
}

// syncChatProjectAgentUser makes the agent user's display name and avatar
// match the agent's name and icon, so the agent shows under its own name
// wherever users are listed. store should be the transaction that changed
// the agent. The username stays stable so references to it keep working.
// Agent names are validated as display names, so they are copied as is.
func syncChatProjectAgentUser(ctx context.Context, store database.Store, project database.ChatProject) error {
	if !project.AgentUserID.Valid {
		return nil
	}
	//nolint:gocritic // Agent users are system-managed.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	user, err := store.GetUserByID(sysCtx, project.AgentUserID.UUID)
	if err != nil {
		return xerrors.Errorf("get agent user: %w", err)
	}
	if user.Name == project.Name && user.AvatarURL == project.Icon {
		return nil
	}
	_, err = store.UpdateUserProfile(sysCtx, database.UpdateUserProfileParams{
		ID:        user.ID,
		Email:     user.Email,
		Username:  user.Username,
		AvatarURL: project.Icon,
		UpdatedAt: dbtime.Now(),
		Name:      project.Name,
	})
	if err != nil {
		return xerrors.Errorf("update agent user: %w", err)
	}
	return nil
}

// syncChatProjectFromAgentUser copies an agent user's new display name and
// avatar to its agent, so renaming the user renames the agent. store should
// be the transaction that updated the user. It does nothing for users that
// are not an agent's user.
func syncChatProjectFromAgentUser(ctx context.Context, store database.Store, user database.User) error {
	if !user.IsAgent {
		return nil
	}
	_, err := store.UpdateChatProjectProfileByAgentUserID(ctx, database.UpdateChatProjectProfileByAgentUserIDParams{
		AgentUserID: user.ID,
		Name:        user.Name,
		Icon:        user.AvatarURL,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("update agent from its user: %w", err)
	}
	return nil
}

// validateChatProjectAgentUserProfile returns the field and message of the
// first profile value an agent user cannot have, because it must also be a
// valid agent name and icon.
func validateChatProjectAgentUserProfile(name, avatarURL string) (field, message string) {
	if strings.TrimSpace(name) == "" {
		return "name", "An agent's name is required."
	}
	if utf8.RuneCountInString(name) > chatProjectNameMaxChars {
		return "name", fmt.Sprintf("An agent's name must be at most %d characters.", chatProjectNameMaxChars)
	}
	if utf8.RuneCountInString(avatarURL) > chatProjectIconMaxChars {
		return "avatar_url", fmt.Sprintf("An agent's avatar URL must be at most %d characters.", chatProjectIconMaxChars)
	}
	return "", ""
}

// suspendChatProjectAgentUser suspends a deleted agent's user. The user is
// kept so audit entries, chats, and workspaces stay attributed to it.
func suspendChatProjectAgentUser(ctx context.Context, store database.Store, project database.ChatProject) error {
	if !project.AgentUserID.Valid {
		return nil
	}
	//nolint:gocritic // Agent users are system-managed.
	_, err := store.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
		ID:         project.AgentUserID.UUID,
		Status:     database.UserStatusSuspended,
		UpdatedAt:  dbtime.Now(),
		UserIsSeen: false,
	})
	if err != nil {
		return xerrors.Errorf("suspend agent user: %w", err)
	}
	return nil
}

// ensureChatProjectAgentUser returns the project with its agent user,
// creating the user for agents made before agents had one. The row lock
// makes concurrent callers, including other replicas, create at most one.
func (api *API) ensureChatProjectAgentUser(ctx context.Context, project database.ChatProject) (database.ChatProject, error) {
	if project.AgentUserID.Valid {
		return project, nil
	}
	//nolint:gocritic // Provisioning links a system-managed account.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	var updated database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		current, err := tx.GetChatProjectByIDForUpdate(sysCtx, project.ID)
		if err != nil {
			return xerrors.Errorf("lock chat project: %w", err)
		}
		if current.AgentUserID.Valid {
			updated = current
			return nil
		}
		user, err := api.createChatProjectAgentUser(ctx, tx, current.OrganizationID, current.Name, current.Icon)
		if err != nil {
			return err
		}
		updated, err = tx.UpdateChatProjectAgentUserID(sysCtx, database.UpdateChatProjectAgentUserIDParams{
			ID:          current.ID,
			AgentUserID: user.ID,
		})
		if err != nil {
			return xerrors.Errorf("link agent user: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return database.ChatProject{}, err
	}
	return updated, nil
}

// backfillChatProjectAgentUsers gives every existing agent its user. It runs
// once at startup; chat creation also provisions on demand.
func (api *API) backfillChatProjectAgentUsers(ctx context.Context) {
	//nolint:gocritic // Provisioning spans every owner's agents.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	ids, err := api.Database.GetChatProjectIDsWithoutAgentUser(sysCtx)
	if err != nil {
		if ctx.Err() == nil {
			api.Logger.Warn(ctx, "list agents without a user", slog.Error(err))
		}
		return
	}
	for _, id := range ids {
		project, err := api.Database.GetChatProjectByID(sysCtx, id)
		if err == nil {
			_, err = api.ensureChatProjectAgentUser(ctx, project)
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			api.Logger.Warn(ctx, "provision agent user", slog.F("project_id", id), slog.Error(err))
		}
	}
	// Agents made before their names were kept in step with their users
	// may have drifted.
	if err := api.Database.SyncChatProjectAgentUserProfiles(sysCtx); err != nil && ctx.Err() == nil {
		api.Logger.Warn(ctx, "sync agent user profiles", slog.Error(err))
	}
}

// chatProjectAgentError reports why an agent's user cannot run a chat.
type chatProjectAgentError struct {
	message string
}

func (e *chatProjectAgentError) Error() string {
	return e.message
}

// chatProjectAgentContext returns the project with its agent user and a
// context that acts as that user, after checking the user can run chats.
func (api *API) chatProjectAgentContext(ctx context.Context, project database.ChatProject) (database.ChatProject, context.Context, error) {
	if api.chatSharingDisabled() {
		// Chats are owned by the agent and shared with the people who
		// start them, so agents cannot work without chat sharing.
		return project, nil, &chatProjectAgentError{message: "Agents require chat sharing, which is disabled for this deployment."}
	}
	project, err := api.ensureChatProjectAgentUser(ctx, project)
	if err != nil {
		return project, nil, xerrors.Errorf("provision agent user: %w", err)
	}
	agentUserID := project.AgentUserID.UUID
	//nolint:gocritic // Agent users are system-managed.
	memberships, err := api.Database.OrganizationMembers(dbauthz.AsSystemRestricted(ctx), database.OrganizationMembersParams{
		OrganizationID: project.OrganizationID,
		UserID:         agentUserID,
		IncludeSystem:  false,
		GithubUserID:   0,
	})
	if err != nil {
		return project, nil, xerrors.Errorf("get agent user membership: %w", err)
	}
	if len(memberships) == 0 || memberships[0].Status != database.UserStatusActive {
		return project, nil, &chatProjectAgentError{message: "The agent's user is not an active member of the organization."}
	}
	subject, _, err := httpmw.UserRBACSubject(ctx, api.Database, agentUserID, rbac.ScopeAll)
	if err != nil {
		return project, nil, xerrors.Errorf("load agent user authorization: %w", err)
	}
	agentCtx := dbauthz.As(ctx, subject)
	chatObject := rbac.ResourceChat.WithOwner(agentUserID.String()).InOrg(project.OrganizationID)
	for _, action := range []policy.Action{policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionShare} {
		if !api.HTTPAuth.AuthorizeContext(agentCtx, action, chatObject) {
			return project, nil, &chatProjectAgentError{message: "The agent's user does not have permission to use chats."}
		}
	}
	return project, agentCtx, nil
}

// shareChatProjectAgentChat lets a person keep working in a chat the agent
// owns. agentCtx must act as the agent user.
func shareChatProjectAgentChat(agentCtx context.Context, store database.Store, chatID, userID uuid.UUID) error {
	err := store.UpdateChatACLByID(agentCtx, database.UpdateChatACLByIDParams{
		ID: chatID,
		UserACL: database.ChatACL{
			userID.String(): {Permissions: db2sdk.ChatRoleActions(codersdk.ChatRoleWrite)},
		},
		GroupACL: database.ChatACL{},
	})
	if err != nil {
		return xerrors.Errorf("share agent chat: %w", err)
	}
	return nil
}

// agentChatMCPServerIDs returns the MCP servers of the agent whose user owns
// the chat. ok is false when a person owns the chat. A chat owned by an
// agent's user runs with the agent's stored credentials, so it may only use
// the servers the agent's owner attached.
func (api *API) agentChatMCPServerIDs(ctx context.Context, ownerID uuid.UUID) (ids []uuid.UUID, ok bool, err error) {
	//nolint:gocritic // The agent is read only to limit the chat to its servers.
	project, err := api.Database.GetChatProjectByAgentUserID(dbauthz.AsSystemRestricted(ctx), ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, xerrors.Errorf("get agent for chat owner: %w", err)
	}
	return project.MCPServerIDs, true, nil
}

// canDriveChat reports whether userID may send messages and run other
// actions that spend the chat owner's credentials. Owners always can. A chat
// owned by an agent's user acts with the agent's credentials, so anyone the
// chat is shared with for writing who can still use the agent can drive it.
// Callers must already have authorized update on the chat.
func (api *API) canDriveChat(ctx context.Context, userID uuid.UUID, chat database.Chat) (bool, error) {
	if chat.OwnerID == userID {
		return true, nil
	}
	if !chat.ProjectID.Valid {
		return false, nil
	}
	//nolint:gocritic // The project is read only to compare its agent user.
	project, err := api.Database.GetChatProjectByID(dbauthz.AsSystemRestricted(ctx), chat.ProjectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("get chat project: %w", err)
	}
	if project.AgentUserID != (uuid.NullUUID{UUID: chat.OwnerID, Valid: true}) {
		return false, nil
	}
	return chatProjectAccessibleByUser(ctx, api.Database, project, userID)
}

// requireDriveChat writes a 403 with message and returns false when userID
// may not drive the chat. See canDriveChat.
func (api *API) requireDriveChat(ctx context.Context, rw http.ResponseWriter, userID uuid.UUID, chat database.Chat, message string) bool {
	ok, err := api.canDriveChat(ctx, userID, chat)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return false
	}
	if !ok {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: message})
		return false
	}
	return true
}
