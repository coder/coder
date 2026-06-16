package policy

import (
	"context"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
)

// policyPackage is the Rego package every gateway policy is authored in. Each
// kind binds a well-known rule within it: verdict (decide), annotations
// (annotate), model (route), body (transform).
const policyPackage = "gateway"

// hermeticCapabilities is the OPA capability set every gateway policy is
// compiled and evaluated against. It is the base set for this OPA version with
// every non-deterministic builtin removed, so a policy physically cannot make a
// network call (http.send), read the clock (time.now_ns), draw randomness
// (rand.intn, uuid.rfc4122), or resolve DNS (net.lookup_ip_addr): such a policy
// fails to compile at the validation gate and fails to prepare at load, rather
// than evaluating non-deterministically. OPA tags these builtins
// Nondeterministic, so that flag is the lever. Hermeticity is thus enforced, not
// assumed (design doc §2).
var hermeticCapabilities = newHermeticCapabilities()

func newHermeticCapabilities() *ast.Capabilities {
	caps := ast.CapabilitiesForThisVersion()
	allowed := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, b := range caps.Builtins {
		if b.Nondeterministic {
			continue
		}
		allowed = append(allowed, b)
	}
	caps.Builtins = allowed
	return caps
}

// ruleQuery returns the prepared-query reference for a kind's entrypoint rule.
func ruleQuery(rule string) string {
	return "data." + policyPackage + "." + rule
}

// preparedQuery is a compiled, reusable policy query (compile-once, eval-many).
type preparedQuery = rego.PreparedEvalQuery

// FailMode controls behavior when a policy cannot produce a result.
type FailMode int

const (
	// FailClosed denies the request (BLOCK) on evaluation error. Default.
	FailClosed FailMode = iota
	// FailOpen skips the failing stage and continues.
	FailOpen
)

// Option configures a policy kind.
type Option func(*options)

type options struct {
	failMode FailMode
}

// WithFailMode overrides the default fail mode (FailClosed).
func WithFailMode(m FailMode) Option {
	return func(o *options) { o.failMode = m }
}

func newOptions(opts ...Option) options {
	o := options{failMode: FailClosed}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// prepare compiles a policy module into a prepared query for its rule.
//
// The package declaration is injected automatically so authors do not need to
// write one. Any package declaration already present in the source is
// overwritten on the parsed AST, so the effective package is always
// policyPackage regardless of what the source says.
func prepare(module, query string) (preparedQuery, error) {
	// Prefix the module source with a placeholder package declaration so
	// ast.ParseModule accepts it; we overwrite it on the AST immediately after.
	src := "package " + policyPackage + "\n\n" + module
	parsed, err := ast.ParseModule("policy.rego", src)
	if err != nil {
		return preparedQuery{}, err
	}
	parsed.Package = &ast.Package{
		Path: ast.MustParseRef("data." + policyPackage),
	}
	pq, err := rego.New(
		rego.Query(query),
		rego.ParsedModule(parsed),
		rego.StrictBuiltinErrors(true),
		// Compile and evaluate against the hermetic capability set so a policy
		// referencing a non-deterministic builtin (http.send, time.now_ns, ...)
		// fails to prepare rather than slipping through.
		rego.Capabilities(hermeticCapabilities),
	).PrepareForEval(context.Background())
	if err != nil {
		return preparedQuery{}, err
	}
	return pq, nil
}

// evalSingle evaluates pq against in and returns the single result value, or
// ok=false when the queried rule is undefined.
func evalSingle(ctx context.Context, pq rego.PreparedEvalQuery, in Input) (any, bool, error) {
	rs, err := pq.Eval(ctx, rego.EvalParsedInput(in.val))
	if err != nil {
		return nil, false, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, false, nil
	}
	return rs[0].Expressions[0].Value, true, nil
}
