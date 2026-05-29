package cli

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatShareCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "share",
		Short: "Manage chat sharing",
		Long:  "Share chats with users and groups.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatShareAddCommand(),
		},
	}
}

const chatShareDefaultGroupDisplay = "-"

type chatRoleLookupParams struct {
	Client      *codersdk.Client
	OrgID       uuid.UUID
	OrgName     string
	Users       [][2]string
	Groups      [][2]string
	DefaultRole codersdk.ChatRole
}

func parseChatShareID(raw string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid chat ID %q: %w", raw, err)
	}
	return parsed, nil
}

func parseChatShareActorRole(raw string) ([2]string, error) {
	if strings.Count(raw, ":") > 1 {
		return [2]string{}, xerrors.New("must match pattern 'name:role'")
	}
	parts := strings.SplitN(raw, ":", 2)
	name := parts[0]
	if name == "" || !codersdk.UsernameValidRegex.MatchString(name) {
		return [2]string{}, xerrors.New("invalid name")
	}
	if len(parts) == 1 {
		return [2]string{name, ""}, nil
	}
	if parts[1] == "" {
		return [2]string{}, xerrors.New("role cannot be empty")
	}
	return [2]string{name, parts[1]}, nil
}

func stringToChatRole(role string) (codersdk.ChatRole, error) {
	switch role {
	case string(codersdk.ChatRoleRead):
		return codersdk.ChatRoleRead, nil
	case string(codersdk.ChatRoleDeleted):
		return codersdk.ChatRoleDeleted, nil
	default:
		return "", xerrors.Errorf("invalid role %q: expected %q", role, codersdk.ChatRoleRead)
	}
}

func fetchChatUsersAndGroups(ctx context.Context, params chatRoleLookupParams) (userRoles map[string]codersdk.ChatRole, groupRoles map[string]codersdk.ChatRole, err error) {
	userRoles = make(map[string]codersdk.ChatRole, len(params.Users))
	if len(params.Users) > 0 {
		orgMembers, err := params.Client.OrganizationMembers(ctx, params.OrgID)
		if err != nil {
			return nil, nil, err
		}

		for _, user := range params.Users {
			username := user[0]
			role := user[1]
			if role == "" {
				role = string(params.DefaultRole)
			}

			userID := ""
			for _, member := range orgMembers {
				if member.Username == username {
					userID = member.UserID.String()
					break
				}
			}
			if userID == "" {
				return nil, nil, xerrors.Errorf("could not find user %s in the organization %s", username, params.OrgName)
			}

			chatRole, err := stringToChatRole(role)
			if err != nil {
				return nil, nil, err
			}
			userRoles[userID] = chatRole
		}
	}

	groupRoles = make(map[string]codersdk.ChatRole, len(params.Groups))
	if len(params.Groups) > 0 {
		orgGroups, err := params.Client.Groups(ctx, codersdk.GroupArguments{
			Organization: params.OrgID.String(),
		})
		if err != nil {
			return nil, nil, err
		}

		for _, group := range params.Groups {
			groupName := group[0]
			role := group[1]
			if role == "" {
				role = string(params.DefaultRole)
			}

			var orgGroup *codersdk.Group
			for _, candidate := range orgGroups {
				if candidate.Name == groupName {
					orgGroup = &candidate
					break
				}
			}
			if orgGroup == nil {
				return nil, nil, xerrors.Errorf("could not find group named %s belonging to the organization %s", groupName, params.OrgName)
			}

			chatRole, err := stringToChatRole(role)
			if err != nil {
				return nil, nil, err
			}
			groupRoles[orgGroup.ID.String()] = chatRole
		}
	}

	return userRoles, groupRoles, nil
}

func chatACLToTable(ctx context.Context, acl *codersdk.ChatACL) (string, error) {
	type chatShareRow struct {
		User  string            `table:"user"`
		Group string            `table:"group,default_sort"`
		Role  codersdk.ChatRole `table:"role"`
	}

	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]chatShareRow{}, []string{"User", "Group", "Role"}),
		cliui.JSONFormat())

	outputRows := make([]chatShareRow, 0, len(acl.Users)+len(acl.Groups))
	for _, user := range acl.Users {
		if user.Role == codersdk.ChatRoleDeleted {
			continue
		}
		outputRows = append(outputRows, chatShareRow{
			User:  user.Username,
			Group: chatShareDefaultGroupDisplay,
			Role:  user.Role,
		})
	}
	for _, group := range acl.Groups {
		if group.Role == codersdk.ChatRoleDeleted {
			continue
		}
		outputRows = append(outputRows, chatShareRow{
			User:  "",
			Group: group.Name,
			Role:  group.Role,
		})
	}

	return formatter.Format(ctx, outputRows)
}
