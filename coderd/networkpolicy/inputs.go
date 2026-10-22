package networkpolicy

import (
	"slices"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"
)

// checkLiteralExpression runs before evaluation. Constant comprehensions can
// expand exponentially despite a small source file, so constants alone are not
// a sufficient bound on the evaluator's work.
func checkLiteralExpression(expr hclsyntax.Expression, depth int) error {
	if depth > 32 {
		return xerrors.New("policy input nesting exceeds 32 levels")
	}
	check := func(child hclsyntax.Expression) error {
		return checkLiteralExpression(child, depth+1)
	}
	switch expr := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return nil
	case *hclsyntax.TemplateExpr:
		if !expr.IsStringLiteral() {
			return xerrors.New("policy strings must not contain interpolation")
		}
	case *hclsyntax.TupleConsExpr:
		for _, item := range expr.Exprs {
			if err := check(item); err != nil {
				return err
			}
		}
	case *hclsyntax.ObjectConsExpr:
		keys := map[string]bool{}
		for _, item := range expr.Items {
			key, ok := item.KeyExpr.(*hclsyntax.ObjectConsKeyExpr)
			if !ok {
				return xerrors.New("policy object keys must be literal strings")
			}
			traversal, bareKey := key.Wrapped.(*hclsyntax.ScopeTraversalExpr)
			if !bareKey || key.ForceNonLiteral || len(traversal.Traversal) != 1 {
				if err := check(key.Wrapped); err != nil {
					return err
				}
			}
			value, diags := key.Value(nil)
			if diags.HasErrors() || value.IsNull() || !value.Type().Equals(cty.String) {
				return xerrors.New("policy object keys must be literal strings")
			}
			if keys[value.AsString()] {
				return xerrors.Errorf("duplicate policy object key %q", value.AsString())
			}
			keys[value.AsString()] = true
			if err := check(item.ValueExpr); err != nil {
				return err
			}
		}
	default:
		return xerrors.New("policy inputs accept only literal strings, numbers, booleans, lists, and objects")
	}
	return nil
}

func checkInput(name string, value cty.Value) error {
	switch name {
	case "source":
		if !value.Type().Equals(cty.String) || value.AsString() != "./module" {
			return xerrors.Errorf("policy module source must be %q", "./module")
		}
	case "direct_p2p_enabled":
		if !value.Type().Equals(cty.Bool) {
			return xerrors.New("direct_p2p_enabled must be a boolean")
		}
	case "allowed_https_hosts", "dns_query_names", "direct_p2p_cidrs":
		return checkStringList(value)
	case "control_plane_allowances", "relay_allowances":
		if !value.Type().IsTupleType() {
			return xerrors.New("allowances must be a list")
		}
		for _, allowance := range value.AsValueSlice() {
			if err := checkObjectKeys(allowance, "endpoint_selectors", "cidrs", "ports"); err != nil {
				return err
			}
			if allowance.Type().HasAttribute("cidrs") {
				if err := checkStringList(allowance.GetAttr("cidrs")); err != nil {
					return err
				}
			}
			if allowance.Type().HasAttribute("endpoint_selectors") {
				selectors := allowance.GetAttr("endpoint_selectors")
				if selectors.IsNull() || !selectors.Type().IsTupleType() {
					return xerrors.New("endpoint_selectors must be a list of label maps")
				}
				for _, selector := range selectors.AsValueSlice() {
					if selector.IsNull() || !selector.Type().IsObjectType() {
						return xerrors.New("endpoint selector must be a label map")
					}
					for _, label := range selector.AsValueMap() {
						if label.IsNull() || !label.Type().Equals(cty.String) {
							return xerrors.New("endpoint selector label values must be strings")
						}
					}
				}
			}
			if !allowance.Type().HasAttribute("ports") {
				return xerrors.New("allowance requires ports")
			}
			ports := allowance.GetAttr("ports")
			if ports.IsNull() || !ports.Type().IsTupleType() {
				return xerrors.New("ports must be a list")
			}
			for _, port := range ports.AsValueSlice() {
				if err := checkObjectKeys(port, "port", "protocol"); err != nil {
					return err
				}
				if !port.Type().HasAttribute("port") || port.GetAttr("port").IsNull() || !port.GetAttr("port").Type().Equals(cty.Number) {
					return xerrors.New("port requires a numeric port field")
				}
				if port.Type().HasAttribute("protocol") && (port.GetAttr("protocol").IsNull() || !port.GetAttr("protocol").Type().Equals(cty.String)) {
					return xerrors.New("port protocol must be a string")
				}
			}
		}
	}
	return nil
}

func checkStringList(value cty.Value) error {
	if value.IsNull() || !value.Type().IsTupleType() {
		return xerrors.New("expected a list of strings")
	}
	for _, item := range value.AsValueSlice() {
		if item.IsNull() || !item.Type().Equals(cty.String) {
			return xerrors.New("expected a list of non-null strings")
		}
	}
	return nil
}

func checkObjectKeys(value cty.Value, allowed ...string) error {
	if value.IsNull() || !value.Type().IsObjectType() {
		return xerrors.New("expected an object")
	}
	for name := range value.Type().AttributeTypes() {
		if !slices.Contains(allowed, name) {
			return xerrors.Errorf("unsupported policy field %q", name)
		}
	}
	return nil
}
