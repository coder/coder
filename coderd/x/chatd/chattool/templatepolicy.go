package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// TemplatePolicy restricts the templates a chat may use, as configured on
// the chat's agent. The zero value allows every template.
type TemplatePolicy struct {
	// AllowedTemplateIDs, when non-empty, lists the only templates the chat
	// may use.
	AllowedTemplateIDs []uuid.UUID
	// AllowedExternalAuthProviderIDs, when non-empty, lists the only
	// external auth providers a template's active version may require.
	AllowedExternalAuthProviderIDs []string
	// ParameterRules maps a template ID to parameter names and the only
	// values the chat may use for them. The first value is the default.
	ParameterRules map[uuid.UUID]map[string][]string
}

// IsZero reports whether the policy allows every template.
func (p TemplatePolicy) IsZero() bool {
	return len(p.AllowedTemplateIDs) == 0 && len(p.AllowedExternalAuthProviderIDs) == 0
}

// parameterRules returns the rules for a template, or nil.
func (p TemplatePolicy) parameterRules(templateID uuid.UUID) map[string][]string {
	return p.ParameterRules[templateID]
}

// CheckParameterValues returns a reason when a supplied parameter value is
// not allowed for the template, or "" when every value is allowed.
func (p TemplatePolicy) CheckParameterValues(templateID uuid.UUID, values map[string]string) string {
	rules := p.parameterRules(templateID)
	for _, name := range slices.Sorted(maps.Keys(values)) {
		allowed, ok := rules[name]
		if ok && !slices.Contains(allowed, values[name]) {
			return disallowedParameterMessage(name, allowed)
		}
	}
	return ""
}

// ApplyParameterRules validates the parameters for a new workspace and
// fills ruled parameters the chat left unset with their first allowed
// value. Preset values override supplied ones when the workspace is built,
// so a preset that sets a ruled parameter to a disallowed value is refused.
// It returns the parameters to send, or a reason the request is not
// allowed.
// ctx must carry an actor that can read the template's active version.
func (p TemplatePolicy) ApplyParameterRules(
	ctx context.Context,
	db database.Store,
	template database.Template,
	presetID uuid.UUID,
	values map[string]string,
) (map[string]string, string) {
	rules := p.parameterRules(template.ID)
	if len(rules) == 0 {
		return values, ""
	}
	if reason := p.CheckParameterValues(template.ID, values); reason != "" {
		return nil, reason
	}

	setByPreset := map[string]bool{}
	if presetID != uuid.Nil {
		presetParams, err := db.GetPresetParametersByPresetID(ctx, presetID)
		if err != nil {
			return nil, "could not verify the preset's parameter values"
		}
		for _, pp := range presetParams {
			if allowed, ok := rules[pp.Name]; ok && !slices.Contains(allowed, pp.Value) {
				return nil, fmt.Sprintf("this agent may not use this preset because it sets parameter %q to a value that is not allowed", pp.Name)
			}
			setByPreset[pp.Name] = true
		}
	}

	versionParams, err := db.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
	if err != nil {
		return nil, "could not verify the template's parameters"
	}
	out := maps.Clone(values)
	if out == nil {
		out = map[string]string{}
	}
	for _, param := range versionParams {
		allowed, ok := rules[param.Name]
		if !ok || len(allowed) == 0 || setByPreset[param.Name] {
			continue
		}
		if _, supplied := out[param.Name]; !supplied {
			out[param.Name] = allowed[0]
		}
	}
	return out, ""
}

func disallowedParameterMessage(name string, allowed []string) string {
	quoted := make([]string, 0, len(allowed))
	for _, v := range allowed {
		quoted = append(quoted, strconv.Quote(v))
	}
	return fmt.Sprintf("this agent may only set parameter %q to one of: %s", name, strings.Join(quoted, ", "))
}

// Check returns a reason the template is not allowed, or "" when it is.
// ctx must carry an actor that can read the template's active version.
func (p TemplatePolicy) Check(ctx context.Context, db database.Store, template database.Template) string {
	if len(p.AllowedTemplateIDs) > 0 && !slices.Contains(p.AllowedTemplateIDs, template.ID) {
		return "this agent is not allowed to use this template; use list_templates to find an allowed template"
	}
	if len(p.AllowedExternalAuthProviderIDs) == 0 {
		return ""
	}
	version, err := db.GetTemplateVersionByID(ctx, template.ActiveVersionID)
	if err != nil {
		return "could not verify the template's external auth providers"
	}
	var providers []database.ExternalAuthProvider
	if len(version.ExternalAuthProviders) > 0 {
		if err := json.Unmarshal(version.ExternalAuthProviders, &providers); err != nil {
			return "could not verify the template's external auth providers"
		}
	}
	for _, provider := range providers {
		if !slices.Contains(p.AllowedExternalAuthProviderIDs, provider.ID) {
			return fmt.Sprintf("this agent is not allowed to use templates that require the %q external auth provider", provider.ID)
		}
	}
	return ""
}

// filterTemplates drops templates the policy does not allow.
func (p TemplatePolicy) filterTemplates(ctx context.Context, db database.Store, templates []database.Template) []database.Template {
	if p.IsZero() {
		return templates
	}
	return slices.DeleteFunc(templates, func(t database.Template) bool {
		return p.Check(ctx, db, t) != ""
	})
}
