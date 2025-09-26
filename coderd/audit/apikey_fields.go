package audit

import (
	"context"
	"encoding/json"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
)

type APIKeyAuditFields struct {
	ID             string                   `json:"id"`
	TokenName      string                   `json:"token_name,omitempty"`
	Scopes         []string                 `json:"scopes,omitempty"`
	AllowList      []string                 `json:"allow_list,omitempty"`
	EffectiveScope *APIEffectiveScopeFields `json:"effective_scope,omitempty"`
}

type APIEffectiveScopeFields struct {
	AllowList []string            `json:"allow_list,omitempty"`
	Site      []string            `json:"site_permissions,omitempty"`
	Org       map[string][]string `json:"org_permissions,omitempty"`
	User      []string            `json:"user_permissions,omitempty"`
}

func APIKeyFields(ctx context.Context, log slog.Logger, key database.APIKey) APIKeyAuditFields {
	fields := APIKeyAuditFields{
		ID:        key.ID,
		TokenName: key.TokenName,
		Scopes:    apiKeyScopesToStrings(key.Scopes),
		AllowList: allowListToStrings(key.AllowList),
	}

	expanded, err := key.ScopeSet().Expand()
	if err != nil {
		log.Warn(ctx, "expand api key effective scope", slog.Error(err))
		return fields
	}

	fields.EffectiveScope = &APIEffectiveScopeFields{
		AllowList: allowListElementsToStrings(expanded.AllowIDList),
		Site:      permissionsToStrings(expanded.Site),
		Org:       orgPermissionsToStrings(expanded.Org),
		User:      permissionsToStrings(expanded.User),
	}

	return fields
}

func WrapAPIKeyFields(fields APIKeyAuditFields) map[string]any {
	return map[string]any{"api_key": fields}
}

func mergeAdditionalFields(ctx context.Context, log slog.Logger, existing json.RawMessage, apiKeyFields APIKeyAuditFields) json.RawMessage {
	base := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &base); err != nil {
			log.Warn(ctx, "unmarshal audit additional fields", slog.Error(err))
			base = map[string]any{}
		}
	}

	base["request_api_key"] = apiKeyFields

	merged, err := json.Marshal(base)
	if err != nil {
		log.Warn(ctx, "marshal audit additional fields", slog.Error(err))
		return existing
	}

	return json.RawMessage(merged)
}

func apiKeyScopesToStrings(scopes database.APIKeyScopes) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, string(scope))
	}
	return out
}

func allowListToStrings(list database.AllowList) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, entry := range list {
		out = append(out, entry.String())
	}
	return out
}

func allowListElementsToStrings(list []rbac.AllowListElement) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, entry := range list {
		out = append(out, entry.String())
	}
	return out
}

func permissionsToStrings(perms []rbac.Permission) []string {
	if len(perms) == 0 {
		return nil
	}
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, perm.ResourceType+":"+string(perm.Action))
	}
	return out
}

func orgPermissionsToStrings(perms map[string][]rbac.Permission) map[string][]string {
	if len(perms) == 0 {
		return nil
	}
	out := make(map[string][]string, len(perms))
	for orgID, list := range perms {
		out[orgID] = permissionsToStrings(list)
	}
	return out
}
