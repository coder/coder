package policy

import (
	"github.com/open-policy-agent/opa/v1/ast"
	"golang.org/x/xerrors"
)

// Kind identifies a policy's role, which determines the Rego entrypoint rule it
// must bind and the host applier that consumes its result.
type Kind string

const (
	KindAnnotate  Kind = "annotate"
	KindRoute     Kind = "route"
	KindDecide    Kind = "decide"
	KindTransform Kind = "transform"
)

// Hook identifies where in the request lifecycle a policy stage runs. The hook
// determines the input envelope and, in turn, which kinds are valid.
type Hook string

const (
	HookPreAuth Hook = "pre_auth"
	HookPreReq  Hook = "pre_req"
	// HookPreTool fires once per assembled, client-bound tool call, before the
	// call is released to the client. Only annotate and decide are valid: the
	// request is already dispatched (no route) and a flushed stream cannot be
	// rewritten (no transform).
	HookPreTool Hook = "pre_tool"
)

// KindValidAtHook reports whether a policy of kind k may run at hook h. An
// unknown hook permits nothing. This mirrors the kind-validity matrix in the
// policy engine design doc (§3) and is enforced both at registration and
// defensively at load.
func KindValidAtHook(h Hook, k Kind) bool {
	switch h {
	case HookPreAuth:
		return k == KindAnnotate || k == KindDecide
	case HookPreReq:
		return k == KindAnnotate || k == KindRoute || k == KindDecide || k == KindTransform
	case HookPreTool:
		return k == KindAnnotate || k == KindDecide
	default:
		return false
	}
}

// CurrentOutputSchemaVersion is the output-contract generation new policy
// versions are stamped with at create/edit (stored in
// ai_gateway_policy_versions.output_schema_version). Like the input stamp
// (CurrentInputSchemaVersion) it is forensic, not a runtime selector: the host
// always consumes the current output contract. The contract per kind (the
// entrypoint rule plus the accepted output type) is kept backward compatible by
// the output shape guard in output_guard_test.go; it may be widened (accept an
// additional shape, which bumps this) but never narrowed. Tightening what the
// host accepts would break already-deployed policies.
const CurrentOutputSchemaVersion = SchemaV1

// EntrypointRule returns the Rego rule a policy of the given kind must define
// within the gateway package. ok is false for an unknown kind.
func EntrypointRule(k Kind) (rule string, ok bool) {
	switch k {
	case KindAnnotate:
		return "annotations", true
	case KindRoute:
		return "model", true
	case KindDecide:
		return "verdict", true
	case KindTransform:
		return "body", true
	default:
		return "", false
	}
}

// Validate compiles module and asserts it actually binds the entrypoint rule for
// its declared kind. Rego is dynamically typed, so an undefined rule (a typo, or
// a policy authored as the wrong kind) otherwise compiles cleanly and silently
// evaluates to "no result", which fails open. For decide policies it also
// requires a `default verdict` rule so an unmatched request cannot fall through
// to an undefined (allow) verdict.
//
// Validate is the registration-time gate; it runs in-process and never shells
// out to the opa CLI.
//
// TODO: extend with per-hook/per-kind JSON schema checks (ast.NewCompiler().
// WithSchemas) and output-shape conformance once the schema registry is authored.
func Validate(kind Kind, module string) error {
	rule, ok := EntrypointRule(kind)
	if !ok {
		return xerrors.Errorf("unknown policy kind %q", kind)
	}

	src := "package " + policyPackage + "\n\n" + module
	parsed, err := ast.ParseModule("policy.rego", src)
	if err != nil {
		return xerrors.Errorf("parse policy: %w", err)
	}

	compiler := ast.NewCompiler()
	compiler.Compile(map[string]*ast.Module{"policy.rego": parsed})
	if compiler.Failed() {
		return xerrors.Errorf("compile policy: %w", compiler.Errors)
	}

	rules := compiler.GetRules(ast.MustParseRef(ruleQuery(rule)))
	if len(rules) == 0 {
		return xerrors.Errorf("policy of kind %q must define a %q rule", kind, rule)
	}

	if kind == KindDecide {
		hasDefault := false
		for _, r := range rules {
			if r.Default {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			return xerrors.Errorf("decide policy must define a `default %s` rule to avoid failing open on an undefined verdict", rule)
		}
	}

	return nil
}
