package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "chat",
		Short: "Manage agent chats",
		Long:  "Commands for interacting with chats from within a workspace.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextCommand(),
			r.chatShareCommand(),
		},
	}
}

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
			r.chatShareRemoveCommand(),
		},
	}
}

func (r *RootCmd) chatShareAddCommand() *serpent.Command {
	var users []string
	var groups []string

	return &serpent.Command{
		Use:   "add <chat-id> --user <user>:<role> --group <group>:<role>",
		Short: "Share a chat with a user or group.",
		Options: serpent.OptionSet{
			{
				Name:        "user",
				Description: "A comma separated list of users to share the chat with.",
				Flag:        "user",
				Value:       serpent.StringArrayOf(&users),
			}, {
				Name:        "group",
				Description: "A comma separated list of groups to share the chat with.",
				Flag:        "group",
				Value:       serpent.StringArrayOf(&groups),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			if len(users) == 0 && len(groups) == 0 {
				return xerrors.New("at least one user or group must be provided")
			}

			chatID, err := parseChatShareID(inv.Args[0])
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			experimentalClient := codersdk.NewExperimentalClient(client)

			chat, err := experimentalClient.GetChat(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("unable to fetch chat %s: %w", inv.Args[0], err)
			}

			userRoleStrings := make([][2]string, len(users))
			for i, user := range users {
				parsed, err := parseChatShareActorRole(user)
				if err != nil {
					return xerrors.Errorf("invalid user format %q: %w", user, err)
				}
				userRoleStrings[i] = parsed
			}

			groupRoleStrings := make([][2]string, len(groups))
			for i, group := range groups {
				parsed, err := parseChatShareActorRole(group)
				if err != nil {
					return xerrors.Errorf("invalid group format %q: %w", group, err)
				}
				groupRoleStrings[i] = parsed
			}

			userRoles, groupRoles, err := fetchChatUsersAndGroups(inv.Context(), chatRoleLookupParams{
				Client:      client,
				OrgID:       chat.OrganizationID,
				Users:       userRoleStrings,
				Groups:      groupRoleStrings,
				DefaultRole: codersdk.ChatRoleRead,
			})
			if err != nil {
				return err
			}

			if err := experimentalClient.UpdateChatACL(inv.Context(), chat.ID, codersdk.UpdateChatACL{
				UserRoles:  userRoles,
				GroupRoles: groupRoles,
			}); err != nil {
				return err
			}

			acl, err := experimentalClient.GetChatACL(inv.Context(), chat.ID)
			if err != nil {
				return xerrors.Errorf("could not fetch current chat ACL after sharing: %w", err)
			}
			out, err := chatACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
}

func (r *RootCmd) chatShareRemoveCommand() *serpent.Command {
	var users []string
	var groups []string

	return &serpent.Command{
		Use:   "remove <chat-id> --user <user> --group <group>",
		Short: "Remove shared access for users or groups from a chat.",
		Options: serpent.OptionSet{
			{
				Name:        "user",
				Description: "A comma separated list of users to remove shared chat access from.",
				Flag:        "user",
				Value:       serpent.StringArrayOf(&users),
			}, {
				Name:        "group",
				Description: "A comma separated list of groups to remove shared chat access from.",
				Flag:        "group",
				Value:       serpent.StringArrayOf(&groups),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			if len(users) == 0 && len(groups) == 0 {
				return xerrors.New("at least one user or group must be provided")
			}

			chatID, err := parseChatShareID(inv.Args[0])
			if err != nil {
				return err
			}

			userRoleStrings := make([][2]string, len(users))
			for i, user := range users {
				parsed, err := parseChatShareActor(user)
				if err != nil {
					return xerrors.Errorf("invalid user format %q: %w", user, err)
				}
				userRoleStrings[i] = parsed
			}

			groupRoleStrings := make([][2]string, len(groups))
			for i, group := range groups {
				parsed, err := parseChatShareActor(group)
				if err != nil {
					return xerrors.Errorf("invalid group format %q: %w", group, err)
				}
				groupRoleStrings[i] = parsed
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			experimentalClient := codersdk.NewExperimentalClient(client)

			chat, err := experimentalClient.GetChat(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("unable to fetch chat %s: %w", inv.Args[0], err)
			}

			userRoles, groupRoles, err := fetchChatUsersAndGroups(inv.Context(), chatRoleLookupParams{
				Client:      client,
				OrgID:       chat.OrganizationID,
				Users:       userRoleStrings,
				Groups:      groupRoleStrings,
				DefaultRole: codersdk.ChatRoleDeleted,
			})
			if err != nil {
				return err
			}

			if err := experimentalClient.UpdateChatACL(inv.Context(), chat.ID, codersdk.UpdateChatACL{
				UserRoles:  userRoles,
				GroupRoles: groupRoles,
			}); err != nil {
				return err
			}

			acl, err := experimentalClient.GetChatACL(inv.Context(), chat.ID)
			if err != nil {
				return xerrors.Errorf("could not fetch current chat ACL after sharing: %w", err)
			}
			out, err := chatACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
}

func (r *RootCmd) chatContextCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "context",
		Short: "Manage chat context",
		Long:  "Add or clear context files and skills for an active chat session.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextAddCommand(),
			r.chatContextClearCommand(),
		},
	}
}

func (*RootCmd) chatContextAddCommand() *serpent.Command {
	var (
		dir    string
		chatID string
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "add",
		Short: "Add context to an active chat",
		Long: "Read instruction files and discover skills from a directory, then add " +
			"them as context to an active chat session. Multiple calls " +
			"are additive.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			if dir == "" && inv.Environ.Get("CODER") != "true" {
				return xerrors.New("this command must be run inside a Coder workspace (set --dir to override)")
			}

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			resolvedDir := dir
			if resolvedDir == "" {
				resolvedDir, err = os.Getwd()
				if err != nil {
					return xerrors.Errorf("get working directory: %w", err)
				}
			}
			resolvedDir, err = filepath.Abs(resolvedDir)
			if err != nil {
				return xerrors.Errorf("resolve directory: %w", err)
			}
			info, err := os.Stat(resolvedDir)
			if err != nil {
				return xerrors.Errorf("cannot read directory %q: %w", resolvedDir, err)
			}
			if !info.IsDir() {
				return xerrors.Errorf("%q is not a directory", resolvedDir)
			}

			parts := agentcontextconfig.ContextPartsFromDir(resolvedDir)
			if len(parts) == 0 {
				_, _ = fmt.Fprintln(inv.Stderr, "No context files or skills found in "+resolvedDir)
				return nil
			}

			// Resolve chat ID from flag or auto-detect.
			resolvedChatID, err := parseChatID(chatID)
			if err != nil {
				return err
			}

			resp, err := client.AddChatContext(ctx, agentsdk.AddChatContextRequest{
				ChatID: resolvedChatID,
				Parts:  parts,
			})
			if err != nil {
				return xerrors.Errorf("add chat context: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Added %d context part(s) to chat %s\n", resp.Count, resp.ChatID)
			return nil
		},
		Options: serpent.OptionSet{
			{
				Name:        "Directory",
				Flag:        "dir",
				Description: "Directory to read context files and skills from. Defaults to the current working directory.",
				Value:       serpent.StringOf(&dir),
			},
			{
				Name:        "Chat ID",
				Flag:        "chat",
				Env:         "CODER_CHAT_ID",
				Description: "Chat ID to add context to. Auto-detected from CODER_CHAT_ID, the only active chat, or the only top-level active chat.",
				Value:       serpent.StringOf(&chatID),
			},
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

func (*RootCmd) chatContextClearCommand() *serpent.Command {
	var chatID string
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "clear",
		Short: "Clear context from an active chat",
		Long: "Soft-delete all context-file and skill messages from an active chat. " +
			"The next turn will re-fetch default context from the agent.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			resolvedChatID, err := parseChatID(chatID)
			if err != nil {
				return err
			}

			resp, err := client.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{
				ChatID: resolvedChatID,
			})
			if err != nil {
				return xerrors.Errorf("clear chat context: %w", err)
			}

			if resp.ChatID == uuid.Nil {
				_, _ = fmt.Fprintln(inv.Stdout, "No active chats to clear.")
			} else {
				_, _ = fmt.Fprintf(inv.Stdout, "Cleared context from chat %s\n", resp.ChatID)
			}
			return nil
		},
		Options: serpent.OptionSet{{
			Name:        "Chat ID",
			Flag:        "chat",
			Env:         "CODER_CHAT_ID",
			Description: "Chat ID to clear context from. Auto-detected from CODER_CHAT_ID, the only active chat, or the only top-level active chat.",
			Value:       serpent.StringOf(&chatID),
		}},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
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

func parseChatShareActor(raw string) ([2]string, error) {
	if strings.Contains(raw, ":") {
		return [2]string{}, xerrors.New("roles are only accepted by chat share add")
	}
	if raw == "" || !codersdk.UsernameValidRegex.MatchString(raw) {
		return [2]string{}, xerrors.New("invalid name")
	}
	return [2]string{raw, ""}, nil
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

func fetchChatUsersAndGroups(ctx context.Context, params chatRoleLookupParams) (map[string]codersdk.ChatRole, map[string]codersdk.ChatRole, error) {
	userRoles := make(map[string]codersdk.ChatRole, len(params.Users))
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

	groupRoles := make(map[string]codersdk.ChatRole, len(params.Groups))
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

// parseChatID returns the chat UUID from the flag value (which
// serpent already populates from --chat or CODER_CHAT_ID). Returns
// uuid.Nil if empty (the server will auto-detect).
func parseChatID(flagValue string) (uuid.UUID, error) {
	if flagValue == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(flagValue)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid chat ID %q: %w", flagValue, err)
	}
	return parsed, nil
}
