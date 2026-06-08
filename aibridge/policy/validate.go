package policy

import (
	"github.com/open-policy-agent/opa/v1/ast"
	"golang.org/x/xerrors"
)

// Kind identifies a policy's role, which determines the Rego entrypoint rule it
// must bind and the host applier that consumes its result.
type Kind string

const (
	KindClassify  Kind = "classify"
	KindRoute     Kind = "route"
	KindDecide    Kind = "decide"
	KindTransform Kind = "transform"
)

// EntrypointRule returns the Rego rule a policy of the given kind must define
// within the gateway package. ok is false for an unknown kind.
func EntrypointRule(k Kind) (rule string, ok bool) {
	switch k {
	case KindClassify:
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
