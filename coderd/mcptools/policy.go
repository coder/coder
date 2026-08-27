package mcptools

import (
	"slices"

	"github.com/coder/coder/v2/codersdk"
)

// Policy combines the legacy exact-match lists with explicit per-tool rules.
type Policy struct {
	AllowList []string
	DenyList  []string
	Rules     []codersdk.MCPServerToolRule
	Default   string
}

// Allowed reports whether a tool passes every configured policy layer.
func Allowed(policy Policy, toolName string) bool {
	if len(policy.AllowList) > 0 {
		if !slices.Contains(policy.AllowList, toolName) {
			return false
		}
	} else if slices.Contains(policy.DenyList, toolName) {
		return false
	}

	for _, rule := range policy.Rules {
		if rule.Tool == toolName {
			return rule.Enabled
		}
	}

	return policy.Default != "disabled"
}
