package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
)

const registryBaseURL = "https://registry.coder.com"

// simpleTypes are the Terraform types the builder UI can represent.
var simpleTypes = map[string]bool{
	"string": true,
	"number": true,
	"bool":   true,
}

// skipVarNames are variables always excluded from the catalog. These are
// UI-ordering or internal plumbing concerns, not admin-facing config.
var skipVarNames = map[string]bool{
	"order":                 true,
	"coder_app_order":       true,
	"coder_parameter_order": true,
	"group":                 true,
	"slug":                  true,
	"display_name":          true,
	"log_path":              true,
	"install_prefix":        true,
	"share":                 true,
	"subdomain":             true,
}

// fetchModule retrieves a single module from the registry per-module endpoint.
// The id should be "namespace/slug" (e.g. "coder/code-server"); it will be
// URL-encoded for the request path.
func fetchModule(ctx context.Context, baseURL, id string) (registryModule, error) {
	reqURL := baseURL + "/api/modules/" + url.PathEscape(id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return registryModule{}, xerrors.Errorf("creating request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return registryModule{}, xerrors.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return registryModule{}, xerrors.Errorf("GET %s: status %d", reqURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return registryModule{}, xerrors.Errorf("reading response: %w", err)
	}

	var mod registryModule
	if err := json.Unmarshal(body, &mod); err != nil {
		return registryModule{}, xerrors.Errorf("decoding response: %w", err)
	}
	return mod, nil
}

// fetchLatestVersion resolves the latest semver for a module using the
// Terraform protocol versions endpoint.
func fetchLatestVersion(ctx context.Context, baseURL, namespace, slug string) (string, error) {
	reqURL := fmt.Sprintf("%s/terraform_protocol/%s/%s/coder/versions", baseURL, namespace, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", xerrors.Errorf("creating request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", xerrors.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", xerrors.Errorf("GET %s: status %d", reqURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", xerrors.Errorf("reading response: %w", err)
	}

	var versionsResp terraformVersionsResponse
	if err := json.Unmarshal(body, &versionsResp); err != nil {
		return "", xerrors.Errorf("decoding response: %w", err)
	}

	if len(versionsResp.Modules) == 0 || len(versionsResp.Modules[0].Versions) == 0 {
		return "", xerrors.Errorf("no versions found for %s/%s", namespace, slug)
	}

	return latestVersion(versionsResp.Modules[0].Versions)
}

// latestVersion finds the highest semver from a list of version entries.
// The API returns versions without a "v" prefix, so we canonicalize them
// for comparison and strip the prefix before returning.
func latestVersion(entries []struct {
	Version string `json:"version"`
},
) (string, error) {
	var best string
	for _, e := range entries {
		v := e.Version
		// The semver package requires a "v" prefix, but the registry
		// API returns bare versions like "1.5.0".
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}
		if !semver.IsValid(v) {
			continue
		}
		if best == "" || semver.Compare(v, best) > 0 {
			best = v
		}
	}
	if best == "" {
		return "", xerrors.New("no valid semver tags found")
	}
	return strings.TrimPrefix(best, "v"), nil
}

// convertVariables filters and converts registry API variables to the
// catalog schema. It skips internal variables, non-simple types, and
// marks agent_id as computed.
func convertVariables(vars []registryVariable, extraSkip []string) []ModuleVariable {
	skipSet := make(map[string]bool, len(skipVarNames)+len(extraSkip))
	for k := range skipVarNames {
		skipSet[k] = true
	}
	for _, s := range extraSkip {
		skipSet[s] = true
	}

	var result []ModuleVariable
	for _, v := range vars {
		if skipSet[v.Name] {
			continue
		}
		if !simpleTypes[v.Type] {
			continue
		}

		computed := v.Name == "agent_id"
		required := v.Required && !computed

		mv := ModuleVariable{
			Name:        v.Name,
			Type:        v.Type,
			Description: v.Description,
			Required:    required,
			Sensitive:   v.Sensitive,
			Computed:    computed,
		}

		if v.Default != nil {
			raw, err := json.Marshal(v.Default)
			if err == nil {
				mv.Default = raw
			}
		}

		result = append(result, mv)
	}
	return result
}

// normalizeIcon converts registry icon paths to web-servable paths.
// The API returns paths like "/module/code.svg"; we serve them as
// "/icon/code.svg" in the Coder dashboard.
func normalizeIcon(icon string) string {
	if strings.HasPrefix(icon, "/module/") {
		return "/icon/" + strings.TrimPrefix(icon, "/module/")
	}
	return icon
}
