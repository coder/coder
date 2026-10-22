// Package networkpolicy validates the constrained Terraform authoring format
// used for independently published workspace network policies.
package networkpolicy

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/xerrors"
)

// ValidateHCL accepts one local policy module with constant inputs. Coder owns
// the provider configuration, workload identity, and deployment target.
func ValidateHCL(source string) error {
	if len(source) > 1<<20 {
		return xerrors.New("policy HCL exceeds 1 MiB")
	}
	tokens, lexDiags := hclsyntax.LexConfig([]byte(source), "main.tf", hcl.InitialPos)
	if lexDiags.HasErrors() {
		return xerrors.Errorf("lex policy HCL: %s", lexDiags.Error())
	}
	if len(tokens) > 16384 {
		return xerrors.New("policy HCL exceeds 16384 tokens")
	}
	depth := 0
	for _, token := range tokens {
		switch token.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen, hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			depth++
		case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack, hclsyntax.TokenCParen, hclsyntax.TokenTemplateSeqEnd:
			depth--
		}
		if depth > 32 {
			return xerrors.New("policy HCL nesting exceeds 32 levels")
		}
	}
	file, diags := hclsyntax.ParseConfig([]byte(source), "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		return xerrors.Errorf("parse policy HCL: %s", diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(body.Attributes) != 0 || len(body.Blocks) != 1 {
		return xerrors.New("policy must contain exactly one module block")
	}
	module := body.Blocks[0]
	if module.Type != "module" || len(module.Labels) != 1 || module.Labels[0] != "coder_network_policy" || len(module.Body.Blocks) != 0 {
		return xerrors.Errorf("policy must contain module %q without nested blocks", "coder_network_policy")
	}
	allowed := map[string]bool{
		"source":                   true,
		"allowed_https_hosts":      true,
		"dns_query_names":          true,
		"control_plane_allowances": true,
		"relay_allowances":         true,
		"direct_p2p_enabled":       true,
		"direct_p2p_cidrs":         true,
	}
	for name, attr := range module.Body.Attributes {
		if !allowed[name] {
			return xerrors.Errorf("unsupported policy module input %q", name)
		}
		if err := checkLiteralExpression(attr.Expr, 0); err != nil {
			return xerrors.Errorf("policy module input %q: %w", name, err)
		}
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsWhollyKnown() || value.IsNull() {
			return xerrors.Errorf("policy module input %q must be a non-null constant", name)
		}
		if err := checkInput(name, value); err != nil {
			return xerrors.Errorf("policy module input %q: %w", name, err)
		}
	}
	if _, ok := module.Body.Attributes["source"]; !ok {
		return xerrors.Errorf("policy module source must be %q", "./module")
	}
	return nil
}
