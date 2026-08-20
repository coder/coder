package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func (server *Server) loadPrivateMCPServerConfigs(
	ctx context.Context,
	chat database.Chat,
) ([]database.MCPServerConfig, map[uuid.UUID][]string, error) {
	if chat.ParentChatID.Valid ||
		isExploreSubagentMode(chat.Mode) ||
		(chat.PlanMode.Valid && chat.PlanMode.ChatPlanMode == database.ChatPlanModePlan) {
		return nil, nil, nil
	}

	raw, err := server.db.GetChatPrivateMCPServerConfigsByChatID(ctx, chat.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, xerrors.Errorf("get private MCP server configs: %w", err)
	}
	if server.privateMCPHTTPClient == nil {
		return nil, nil, xerrors.New("private MCP server configs require a guarded HTTP client")
	}

	var privateConfigs []codersdk.PrivateMCPServerConfig
	if err := json.Unmarshal(raw, &privateConfigs); err != nil {
		return nil, nil, xerrors.Errorf("parse private MCP server configs: %w", err)
	}
	if len(privateConfigs) > codersdk.MaxPrivateMCPServerConfigs {
		return nil, nil, xerrors.Errorf(
			"private MCP server config count %d exceeds maximum %d",
			len(privateConfigs),
			codersdk.MaxPrivateMCPServerConfigs,
		)
	}

	configs := make([]database.MCPServerConfig, 0, len(privateConfigs))
	sensitiveValues := make(map[uuid.UUID][]string, len(privateConfigs))
	seenNames := make(map[string]struct{}, len(privateConfigs))
	for _, privateConfig := range privateConfigs {
		if privateConfig.Name == "" || privateConfig.URL == "" {
			return nil, nil, xerrors.New("private MCP server config name and URL are required")
		}
		if _, ok := seenNames[privateConfig.Name]; ok {
			return nil, nil, xerrors.Errorf("duplicate private MCP server name %q", privateConfig.Name)
		}
		seenNames[privateConfig.Name] = struct{}{}

		headerJSON, err := json.Marshal(privateConfig.Headers)
		if err != nil {
			return nil, nil, xerrors.Errorf("marshal private MCP server headers: %w", err)
		}
		configID := uuid.NewSHA1(chat.ID, []byte(privateConfig.Name))
		authType := "none"
		if len(privateConfig.Headers) > 0 {
			authType = "custom_headers"
		}
		configs = append(configs, database.MCPServerConfig{
			ID:            configID,
			DisplayName:   privateConfig.Name,
			Slug:          privateConfig.Name,
			Url:           privateConfig.URL,
			Transport:     "streamable_http",
			AuthType:      authType,
			CustomHeaders: string(headerJSON),
			ToolAllowList: slices.Clone(privateConfig.ToolAllowList),
			ToolDenyList:  slices.Clone(privateConfig.ToolDenyList),
			Enabled:       true,
		})

		values := []string{privateConfig.URL}
		for name, value := range privateConfig.Headers {
			values = append(values, name, value)
		}
		sensitiveValues[configID] = values
	}
	return configs, sensitiveValues, nil
}

func appendPrivateMCPTools(
	ctx context.Context,
	logger slog.Logger,
	tools []fantasy.AgentTool,
	privateTools []fantasy.AgentTool,
) []fantasy.AgentTool {
	seen := make(map[string]struct{}, len(tools)+len(privateTools))
	for _, tool := range tools {
		seen[tool.Info().Name] = struct{}{}
	}
	for _, tool := range privateTools {
		name := tool.Info().Name
		if _, ok := seen[name]; ok {
			logger.Warn(ctx, "skipping private MCP tool due to name collision", slog.F("tool_name", name))
			continue
		}
		seen[name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools
}
