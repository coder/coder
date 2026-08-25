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

// Action is a policy disposition for one tool.
type Action string

const (
	// ActionPermit forwards calls and lists the tool.
	ActionPermit Action = "permit"
	// ActionBlock hides the tool and denies calls.
	ActionBlock Action = "block"
	// ActionEscalate holds each call for human approval. Consumers without
	// an escalation path must treat it as ActionBlock.
	ActionEscalate Action = "escalate"
)

// Evaluate resolves a tool's disposition across every configured policy
// layer. The legacy allow and deny lists are binary and apply first: a tool
// they exclude is blocked outright and cannot be escalated into existence.
func Evaluate(policy Policy, toolName string) Action {
	if len(policy.AllowList) > 0 {
		if !slices.Contains(policy.AllowList, toolName) {
			return ActionBlock
		}
	} else if slices.Contains(policy.DenyList, toolName) {
		return ActionBlock
	}

	for _, rule := range policy.Rules {
		if rule.Tool == toolName {
			switch rule.EffectiveAction() {
			case codersdk.MCPServerToolActionEnabled:
				return ActionPermit
			case codersdk.MCPServerToolActionEscalate:
				return ActionEscalate
			default:
				return ActionBlock
			}
		}
	}

	switch policy.Default {
	case "disabled":
		return ActionBlock
	case "escalate":
		return ActionEscalate
	default:
		return ActionPermit
	}
}

// Allowed reports whether a tool is unconditionally permitted. Escalated
// tools are not: callers without an escalation path fail closed.
func Allowed(policy Policy, toolName string) bool {
	return Evaluate(policy, toolName) == ActionPermit
}
