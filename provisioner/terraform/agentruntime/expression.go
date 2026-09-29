package agentruntime

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"
)

type runtimeLocalKey struct {
	moduleAddress string
	name          string
}

type runtimeLocalReference struct {
	reference            string
	correlatedCollection string
}

func (i *configIndex) analyzeRuntimeSourceExpressions(
	ctx context.Context,
	budget *provenanceBudget,
) error {
	clear(i.localRuntime)
	clear(i.runtimeIdentityExpressions)
	clear(i.runtimeEachValueExpressions)
	clear(i.runtimeResultSuffixes)
	clear(i.runtimeValueReferences)

	keys := slices.SortedFunc(
		maps.Keys(i.runtimeSourceExpressions),
		func(a, b runtimeExpressionKey) int {
			return cmp.Or(
				strings.Compare(a.moduleAddress, b.moduleAddress),
				cmp.Compare(a.kind, b.kind),
				strings.Compare(a.resourceType, b.resourceType),
				strings.Compare(a.name, b.name),
				strings.Compare(a.attribute, b.attribute),
			)
		},
	)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		preservesIdentity, usesEachValue, resultSuffix, err :=
			runtimeExpressionMetadata(
				i.runtimeSourceExpressions[key], budget,
			)
		if err != nil {
			return xerrors.Errorf(
				"analyze Terraform expression %q runtime provenance: %w",
				key.attribute, err,
			)
		}
		i.runtimeIdentityExpressions[key] = preservesIdentity
		i.runtimeEachValueExpressions[key] = usesEachValue
		i.runtimeResultSuffixes[key] = resultSuffix
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		references, err := runtimeExpressionReferences(
			ctx,
			i.runtimeSourceExpressions[key],
			nil,
			budget,
			func(expression hcl.Expression, depth int) (bool, error) {
				return i.runtimeCollectionPreservesSourceKeys(
					key.moduleAddress, expression, budget, depth,
				)
			},
		)
		if err != nil {
			return xerrors.Errorf(
				"analyze Terraform expression %q runtime references: %w",
				key.attribute, err,
			)
		}
		i.runtimeValueReferences[key] = runtimeReferenceValues(
			references,
		)
		if key.kind == runtimeExpressionLocal {
			i.localRuntime[runtimeLocalKey{
				moduleAddress: key.moduleAddress,
				name:          key.attribute,
			}] = references
		}
	}
	i.runtimeSourceIndexed = true
	return nil
}

func (i *configIndex) runtimeExpressionPreservesIdentity(
	key runtimeExpressionKey,
) bool {
	if i == nil || !i.runtimeSourceIndexed {
		return true
	}
	return i.runtimeIdentityExpressions[key]
}

func (i *configIndex) runtimeExpressionUsesEachValueAsValue(
	key runtimeExpressionKey,
	references []string,
) bool {
	if i == nil || !i.runtimeSourceIndexed {
		return runtimeReferencesUseEachValueAsValue(references)
	}
	return i.runtimeEachValueExpressions[key]
}

func (i *configIndex) runtimeExpressionResultSuffix(
	key runtimeExpressionKey,
) string {
	if i == nil || !i.runtimeSourceIndexed {
		return ""
	}
	return i.runtimeResultSuffixes[key]
}

func (i *configIndex) runtimeExpressionValueReferences(
	key runtimeExpressionKey,
	fallback []string,
) []string {
	if i == nil || !i.runtimeSourceIndexed {
		return slices.Clone(fallback)
	}
	return slices.Clone(i.runtimeValueReferences[key])
}

func runtimeReferencesUseEachValueAsValue(
	references []string,
) bool {
	usesEachValue := false
	for _, reference := range references {
		suffix, eachValue := strings.CutPrefix(reference, "each.value")
		if eachValue && (suffix == "" || strings.HasPrefix(suffix, ".") ||
			strings.HasPrefix(suffix, "[")) {
			usesEachValue = true
			continue
		}
		if reference != "each" && !strings.HasPrefix(reference, "each.") {
			return false
		}
	}
	return usesEachValue
}

func runtimeReferenceValues(
	references []runtimeLocalReference,
) []string {
	values := make([]string, 0, len(references))
	for _, reference := range references {
		values = append(values, reference.reference)
	}
	return values
}

func runtimeExpressionMetadata(
	expression hcl.Expression,
	budget *provenanceBudget,
) (
	preservesIdentity bool,
	usesEachValue bool,
	resultSuffix string,
	err error,
) {
	preservesIdentity, err = runtimeExpressionPreservesIdentity(
		expression, budget,
	)
	if err != nil {
		return false, false, "", err
	}
	usesEachValue, err = runtimeExpressionUsesEachValueAsValue(
		expression, budget,
	)
	if err != nil {
		return false, false, "", err
	}
	resultSuffix, err = runtimeExpressionResultSuffix(
		expression, budget,
	)
	if err != nil {
		return false, false, "", err
	}
	return preservesIdentity, usesEachValue, resultSuffix, nil
}

func runtimeExpressionUsesEachValueAsValue(
	expression hcl.Expression,
	budget *provenanceBudget,
) (bool, error) {
	return runtimeExpressionUsesEachValueAsValueAtScope(
		expression, nil, budget, 0,
	)
}

func runtimeExpressionUsesEachValueAsValueAtScope(
	expression hcl.Expression,
	iteratorSources map[string]bool,
	budget *provenanceBudget,
	depth int,
) (bool, error) {
	if err := budget.checkDepth(depth); err != nil {
		return false, err
	}
	expression = hcl.UnwrapExpression(expression)
	switch expression := expression.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		if len(expression.Traversal) == 0 {
			return false, nil
		}
		root, ok := expression.Traversal[0].(hcl.TraverseRoot)
		if !ok {
			return false, nil
		}
		if source, iterator := iteratorSources[root.Name]; iterator {
			return source, nil
		}
		if root.Name != "each" || len(expression.Traversal) < 2 {
			return false, nil
		}
		switch step := expression.Traversal[1].(type) {
		case hcl.TraverseAttr:
			return step.Name == "value", nil
		case hcl.TraverseIndex:
			return step.Key.RawEquals(cty.StringVal("value")), nil
		default:
			return false, nil
		}
	case *hclsyntax.ParenthesesExpr:
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Expression, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.TemplateWrapExpr:
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Wrapped, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.RelativeTraversalExpr:
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Source, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.IndexExpr:
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Collection, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.SplatExpr:
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Source, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.ConditionalExpr:
		usesEachValue, err :=
			runtimeExpressionUsesEachValueAsValueAtScope(
				expression.TrueResult, iteratorSources, budget, depth+1,
			)
		if err != nil || usesEachValue {
			return usesEachValue, err
		}
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.FalseResult, iteratorSources, budget, depth+1,
		)
	case *hclsyntax.ForExpr:
		childIteratorSources := maps.Clone(iteratorSources)
		if childIteratorSources == nil {
			childIteratorSources = map[string]bool{}
		}
		if expression.KeyVar != "" {
			childIteratorSources[expression.KeyVar] = false
		}
		collectionUsesEachValue, err :=
			runtimeExpressionUsesEachValueAsValueAtScope(
				expression.CollExpr, iteratorSources, budget, depth+1,
			)
		if err != nil {
			return false, err
		}
		childIteratorSources[expression.ValVar] = collectionUsesEachValue
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.ValExpr, childIteratorSources, budget, depth+1,
		)
	case *hclsyntax.TupleConsExpr:
		for _, child := range expression.Exprs {
			usesEachValue, err :=
				runtimeExpressionUsesEachValueAsValueAtScope(
					child, iteratorSources, budget, depth+1,
				)
			if err != nil || usesEachValue {
				return usesEachValue, err
			}
		}
		return false, nil
	case *hclsyntax.ObjectConsExpr:
		for _, item := range expression.Items {
			usesEachValue, err :=
				runtimeExpressionUsesEachValueAsValueAtScope(
					item.ValueExpr, iteratorSources, budget, depth+1,
				)
			if err != nil || usesEachValue {
				return usesEachValue, err
			}
		}
		return false, nil
	case *hclsyntax.FunctionCallExpr:
		if expression.Name != "tomap" || len(expression.Args) != 1 ||
			expression.ExpandFinal {
			return false, nil
		}
		return runtimeExpressionUsesEachValueAsValueAtScope(
			expression.Args[0], iteratorSources, budget, depth+1,
		)
	default:
		return false, nil
	}
}

func runtimeExpressionPreservesIdentity(
	expression hcl.Expression,
	budget *provenanceBudget,
) (bool, error) {
	return runtimeExpressionPreservesIdentityAtScope(
		expression, nil, budget, 0,
	)
}

func runtimeExpressionPreservesIdentityAtScope(
	expression hcl.Expression,
	iteratorNames map[string]bool,
	budget *provenanceBudget,
	depth int,
) (bool, error) {
	if err := budget.checkDepth(depth); err != nil {
		return false, err
	}
	expression = hcl.UnwrapExpression(expression)
	switch expression := expression.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return runtimeTraversalPreservesIdentity(
			expression.Traversal, iteratorNames,
		), nil
	case *hclsyntax.AnonSymbolExpr:
		return true, nil
	case *hclsyntax.ParenthesesExpr:
		return runtimeExpressionPreservesIdentityAtScope(
			expression.Expression, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.TemplateWrapExpr:
		return runtimeExpressionPreservesIdentityAtScope(
			expression.Wrapped, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.RelativeTraversalExpr:
		if _, ok := hcl.UnwrapExpression(expression.Source).(*hclsyntax.AnonSymbolExpr); ok {
			attribute, ok := expression.Traversal[len(expression.Traversal)-1].(hcl.TraverseAttr)
			return ok && slices.Contains(
				[]string{"agent_id", "id", "subagent_id"}, attribute.Name,
			), nil
		}
		traversal, ok, err := runtimeExpressionTraversal(
			expression, budget, depth,
		)
		if err != nil {
			return false, err
		}
		return ok && runtimeTraversalPreservesIdentity(
			traversal, iteratorNames,
		), nil
	case *hclsyntax.IndexExpr:
		return runtimeExpressionPreservesIdentityAtScope(
			expression.Collection, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.SplatExpr:
		preservesIdentity, err :=
			runtimeExpressionPreservesIdentityAtScope(
				expression.Source, iteratorNames, budget, depth+1,
			)
		if err != nil || !preservesIdentity {
			return preservesIdentity, err
		}
		return runtimeExpressionPreservesIdentityAtScope(
			expression.Each, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.ConditionalExpr:
		preservesIdentity, err :=
			runtimeExpressionPreservesIdentityAtScope(
				expression.TrueResult, iteratorNames, budget, depth+1,
			)
		if err != nil || !preservesIdentity {
			return preservesIdentity, err
		}
		return runtimeExpressionPreservesIdentityAtScope(
			expression.FalseResult, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.ForExpr:
		childIteratorNames := maps.Clone(iteratorNames)
		if childIteratorNames == nil {
			childIteratorNames = map[string]bool{}
		}
		if expression.KeyVar != "" {
			delete(childIteratorNames, expression.KeyVar)
		}
		allowsIndirectAttributes, err :=
			runtimeExpressionAllowsIndirectAttributes(
				expression.CollExpr, budget, depth+1,
			)
		if err != nil {
			return false, err
		}
		childIteratorNames[expression.ValVar] = allowsIndirectAttributes
		preservesIdentity, err :=
			runtimeExpressionPreservesIdentityAtScope(
				expression.CollExpr, iteratorNames, budget, depth+1,
			)
		if err != nil || !preservesIdentity {
			return preservesIdentity, err
		}
		return runtimeExpressionPreservesIdentityAtScope(
			expression.ValExpr, childIteratorNames, budget, depth+1,
		)
	case *hclsyntax.TupleConsExpr:
		return allRuntimeExpressionsPreserveIdentity(
			expression.Exprs, iteratorNames, budget, depth+1,
		)
	case *hclsyntax.ObjectConsExpr:
		for _, item := range expression.Items {
			preservesIdentity, err :=
				runtimeExpressionPreservesIdentityAtScope(
					item.ValueExpr, iteratorNames, budget, depth+1,
				)
			if err != nil || !preservesIdentity {
				return preservesIdentity, err
			}
		}
		return true, nil
	case *hclsyntax.FunctionCallExpr:
		if expression.Name != "tomap" || len(expression.Args) != 1 ||
			expression.ExpandFinal {
			return false, nil
		}
		return runtimeExpressionPreservesIdentityAtScope(
			expression.Args[0], iteratorNames, budget, depth+1,
		)
	default:
		return false, nil
	}
}

func runtimeExpressionTraversal(
	expression hcl.Expression,
	budget *provenanceBudget,
	depth int,
) (hcl.Traversal, bool, error) {
	if err := budget.checkDepth(depth); err != nil {
		return nil, false, err
	}
	expression = hcl.UnwrapExpression(expression)
	switch expression := expression.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return slices.Clone(expression.Traversal), true, nil
	case *hclsyntax.ParenthesesExpr:
		return runtimeExpressionTraversal(
			expression.Expression, budget, depth+1,
		)
	case *hclsyntax.TemplateWrapExpr:
		return runtimeExpressionTraversal(
			expression.Wrapped, budget, depth+1,
		)
	case *hclsyntax.IndexExpr:
		return runtimeExpressionTraversal(
			expression.Collection, budget, depth+1,
		)
	case *hclsyntax.RelativeTraversalExpr:
		traversal, ok, err := runtimeExpressionTraversal(
			expression.Source, budget, depth+1,
		)
		if err != nil || !ok {
			return nil, false, err
		}
		return append(traversal, expression.Traversal...), true, nil
	default:
		return nil, false, nil
	}
}

func runtimeExpressionResultSuffix(
	expression hcl.Expression,
	budget *provenanceBudget,
) (string, error) {
	return runtimeExpressionResultSuffixAtDepth(
		expression, budget, 0,
	)
}

func runtimeExpressionResultSuffixAtDepth(
	expression hcl.Expression,
	budget *provenanceBudget,
	depth int,
) (string, error) {
	if err := budget.checkDepth(depth); err != nil {
		return "", err
	}
	expression = hcl.UnwrapExpression(expression)
	if conditional, ok := expression.(*hclsyntax.ConditionalExpr); ok {
		trueSuffix, err := runtimeExpressionResultSuffixAtDepth(
			conditional.TrueResult, budget, depth+1,
		)
		if err != nil {
			return "", err
		}
		falseSuffix, err := runtimeExpressionResultSuffixAtDepth(
			conditional.FalseResult, budget, depth+1,
		)
		if err != nil {
			return "", err
		}
		if trueSuffix == falseSuffix {
			return trueSuffix, nil
		}
		return "", nil
	}
	traversal, ok, err := runtimeExpressionTraversal(
		expression, budget, depth,
	)
	if err != nil || !ok {
		return "", err
	}
	traversal, _ = runtimeTraversalWithoutResourcePrefix(traversal)
	if len(traversal) < 3 {
		return "", nil
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return "", nil
	}
	switch root.Name {
	case "coder_agent", "coder_devcontainer", "each", "local", "var":
	default:
		return "", nil
	}
	attribute, ok := traversal[len(traversal)-1].(hcl.TraverseAttr)
	if !ok || !slices.Contains(
		[]string{"agent_id", "id", "subagent_id"}, attribute.Name,
	) {
		return "", nil
	}
	return "." + attribute.Name, nil
}

func runtimeExpressionAllowsIndirectAttributes(
	expression hcl.Expression,
	budget *provenanceBudget,
	depth int,
) (bool, error) {
	traversal, ok, err := runtimeExpressionTraversal(
		expression, budget, depth,
	)
	if err != nil || !ok || len(traversal) == 0 {
		return false, err
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return false, nil
	}
	return root.Name == "local" || root.Name == "module" ||
		root.Name == "var", nil
}

func runtimeTraversalWithoutResourcePrefix(
	traversal hcl.Traversal,
) (hcl.Traversal, bool) {
	if len(traversal) < 2 {
		return traversal, false
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok || root.Name != "resource" {
		return traversal, false
	}
	resourceType, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return traversal, false
	}
	normalized := slices.Clone(traversal[1:])
	normalized[0] = hcl.TraverseRoot{
		Name: resourceType.Name, SrcRange: resourceType.SrcRange,
	}
	return normalized, true
}

func runtimeTraversalPreservesIdentity(
	traversal hcl.Traversal,
	iteratorNames map[string]bool,
) bool {
	var explicitResource bool
	if _, resourceIterator := iteratorNames["resource"]; !resourceIterator {
		traversal, explicitResource =
			runtimeTraversalWithoutResourcePrefix(traversal)
	}
	if len(traversal) == 0 {
		return false
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return false
	}
	if explicitResource && root.Name != "coder_agent" &&
		root.Name != "coder_devcontainer" {
		return false
	}
	var allowedAttributes []string
	switch root.Name {
	case "coder_agent":
		_, indexed := traversal[len(traversal)-1].(hcl.TraverseIndex)
		if len(traversal) == 2 || (len(traversal) == 3 && indexed) {
			return true
		}
		allowedAttributes = []string{"id"}
	case "coder_devcontainer":
		_, indexed := traversal[len(traversal)-1].(hcl.TraverseIndex)
		if len(traversal) == 2 || (len(traversal) == 3 && indexed) {
			return true
		}
		allowedAttributes = []string{"agent_id", "subagent_id"}
	case "each":
		if len(traversal) == 2 {
			switch step := traversal[1].(type) {
			case hcl.TraverseAttr:
				return step.Name == "value"
			case hcl.TraverseIndex:
				return step.Key.RawEquals(cty.StringVal("value"))
			default:
				return false
			}
		}
		allowedAttributes = []string{"agent_id", "id", "subagent_id"}
	case "local", "module", "var":
		return true
	default:
		allowsIndirectAttributes, iterator := iteratorNames[root.Name]
		if !iterator {
			return false
		}
		if len(traversal) == 1 || allowsIndirectAttributes {
			return true
		}
		allowedAttributes = []string{"agent_id", "id", "subagent_id"}
	}
	attribute, ok := traversal[len(traversal)-1].(hcl.TraverseAttr)
	return ok && slices.Contains(allowedAttributes, attribute.Name)
}

func allRuntimeExpressionsPreserveIdentity(
	expressions []hclsyntax.Expression,
	iteratorNames map[string]bool,
	budget *provenanceBudget,
	depth int,
) (bool, error) {
	for _, expression := range expressions {
		preservesIdentity, err :=
			runtimeExpressionPreservesIdentityAtScope(
				expression, iteratorNames, budget, depth,
			)
		if err != nil || !preservesIdentity {
			return preservesIdentity, err
		}
	}
	return true, nil
}

func runtimeExpressionReferences(
	ctx context.Context,
	expression hcl.Expression,
	scope map[string][]string,
	budget *provenanceBudget,
	collectionPreservesSourceKeys func(hcl.Expression, int) (bool, error),
) ([]runtimeLocalReference, error) {
	return runtimeExpressionReferencesAtDepth(
		ctx,
		expression,
		scope,
		budget,
		collectionPreservesSourceKeys,
		0,
	)
}

func runtimeExpressionReferencesAtDepth(
	ctx context.Context,
	expression hcl.Expression,
	scope map[string][]string,
	budget *provenanceBudget,
	collectionPreservesSourceKeys func(hcl.Expression, int) (bool, error),
	depth int,
) ([]runtimeLocalReference, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := budget.checkDepth(depth); err != nil {
		return nil, err
	}
	expression = hcl.UnwrapExpression(expression)
	switch wrapped := expression.(type) {
	case *hclsyntax.ParenthesesExpr:
		return runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.Expression,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
	case *hclsyntax.TemplateWrapExpr:
		return runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.Wrapped,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
	case *hclsyntax.RelativeTraversalExpr:
		return runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.Source,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
	case *hclsyntax.FunctionCallExpr:
		if wrapped.Name == "tomap" && len(wrapped.Args) == 1 &&
			!wrapped.ExpandFinal {
			return runtimeExpressionReferencesAtDepth(
				ctx,
				wrapped.Args[0],
				scope,
				budget,
				collectionPreservesSourceKeys,
				depth+1,
			)
		}
	case *hclsyntax.ConditionalExpr:
		trueReferences, err := runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.TrueResult,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
		if err != nil {
			return nil, err
		}
		falseReferences, err := runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.FalseResult,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
		if err != nil {
			return nil, err
		}
		return runtimeMergeLocalReferences(
			trueReferences, falseReferences,
		), nil
	case *hclsyntax.IndexExpr:
		return runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.Collection,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
	case *hclsyntax.SplatExpr:
		return runtimeExpressionReferencesAtDepth(
			ctx,
			wrapped.Source,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
	case *hclsyntax.TupleConsExpr:
		referenceSets := make(
			[][]runtimeLocalReference, 0, len(wrapped.Exprs),
		)
		for _, child := range wrapped.Exprs {
			references, err := runtimeExpressionReferencesAtDepth(
				ctx,
				child,
				scope,
				budget,
				collectionPreservesSourceKeys,
				depth+1,
			)
			if err != nil {
				return nil, err
			}
			referenceSets = append(referenceSets, references)
		}
		return runtimeMergeLocalReferences(referenceSets...), nil
	case *hclsyntax.ObjectConsExpr:
		referenceSets := make(
			[][]runtimeLocalReference, 0, len(wrapped.Items),
		)
		for _, item := range wrapped.Items {
			references, err := runtimeExpressionReferencesAtDepth(
				ctx,
				item.ValueExpr,
				scope,
				budget,
				collectionPreservesSourceKeys,
				depth+1,
			)
			if err != nil {
				return nil, err
			}
			referenceSets = append(referenceSets, references)
		}
		return runtimeMergeLocalReferences(referenceSets...), nil
	}

	if forExpression, ok := expression.(*hclsyntax.ForExpr); ok {
		collectionReferences, err := runtimeExpressionReferencesAtDepth(
			ctx,
			forExpression.CollExpr,
			scope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
		if err != nil {
			return nil, err
		}
		collection := make([]string, 0, len(collectionReferences))
		collectionSet := make(map[string]struct{}, len(collectionReferences))
		for _, reference := range collectionReferences {
			if _, duplicate := collectionSet[reference.reference]; duplicate {
				continue
			}
			collectionSet[reference.reference] = struct{}{}
			collection = append(collection, reference.reference)
		}
		childScope := maps.Clone(scope)
		if childScope == nil {
			childScope = map[string][]string{}
		}
		if forExpression.KeyVar != "" {
			childScope[forExpression.KeyVar] = nil
		}
		childScope[forExpression.ValVar] = collection
		result, err := runtimeExpressionReferencesAtDepth(
			ctx,
			forExpression.ValExpr,
			childScope,
			budget,
			collectionPreservesSourceKeys,
			depth+1,
		)
		if err != nil {
			return nil, err
		}
		preservesKeys, err := runtimeForExpressionPreservesKeys(
			forExpression, collectionPreservesSourceKeys, depth,
		)
		if err != nil {
			return nil, err
		}
		if preservesKeys {
			for index := range result {
				result[index].correlatedCollection =
					runtimeForExpressionCollection(
						result[index].reference, collectionSet,
					)
			}
		}
		return result, nil
	}

	var preservesSourceKeys bool
	if collectionPreservesSourceKeys != nil {
		var err error
		preservesSourceKeys, err = collectionPreservesSourceKeys(
			expression, depth,
		)
		if err != nil {
			return nil, err
		}
	}
	references := map[runtimeLocalReference]struct{}{}
	generatedReferences := 0
	for index, traversal := range expression.Variables() {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		err := visitRuntimeTraversalReferences(
			traversal,
			scope,
			func(reference string) error {
				if generatedReferences%256 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				generatedReferences++
				if err := budget.consumeReference(len(reference)); err != nil {
					return err
				}
				correlatedCollection := ""
				if preservesSourceKeys {
					correlatedCollection = reference
				}
				references[runtimeLocalReference{
					reference:            reference,
					correlatedCollection: correlatedCollection,
				}] = struct{}{}
				return nil
			},
		)
		if err != nil {
			return nil, err
		}
	}
	result := slices.Collect(maps.Keys(references))
	slices.SortFunc(result, func(a, b runtimeLocalReference) int {
		return cmp.Or(
			strings.Compare(a.reference, b.reference),
			strings.Compare(a.correlatedCollection, b.correlatedCollection),
		)
	})
	return result, nil
}

func runtimeMergeLocalReferences(
	referenceSets ...[]runtimeLocalReference,
) []runtimeLocalReference {
	references := map[runtimeLocalReference]struct{}{}
	for _, referenceSet := range referenceSets {
		for _, reference := range referenceSet {
			references[reference] = struct{}{}
		}
	}
	result := slices.Collect(maps.Keys(references))
	slices.SortFunc(result, func(a, b runtimeLocalReference) int {
		return cmp.Or(
			strings.Compare(a.reference, b.reference),
			strings.Compare(a.correlatedCollection, b.correlatedCollection),
		)
	})
	return result
}

func runtimeForExpressionPreservesKeys(
	expression *hclsyntax.ForExpr,
	collectionPreservesSourceKeys func(hcl.Expression, int) (bool, error),
	depth int,
) (bool, error) {
	if expression.KeyExpr == nil || expression.KeyVar == "" ||
		collectionPreservesSourceKeys == nil {
		return false, nil
	}
	preservesSourceKeys, err := collectionPreservesSourceKeys(
		expression.CollExpr, depth+1,
	)
	if err != nil || !preservesSourceKeys {
		return false, err
	}
	key, ok := hcl.UnwrapExpression(
		expression.KeyExpr,
	).(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(key.Traversal) != 1 {
		return false, nil
	}
	root, ok := key.Traversal[0].(hcl.TraverseRoot)
	return ok && root.Name == expression.KeyVar, nil
}

func (i *configIndex) runtimeCollectionPreservesSourceKeys(
	moduleAddress string,
	expression hcl.Expression,
	budget *provenanceBudget,
	depth int,
) (bool, error) {
	if err := budget.checkDepth(depth); err != nil {
		return false, err
	}
	expression = hcl.UnwrapExpression(expression)
	switch expression := expression.(type) {
	case *hclsyntax.ParenthesesExpr:
		return i.runtimeCollectionPreservesSourceKeys(
			moduleAddress, expression.Expression, budget, depth+1,
		)
	case *hclsyntax.TemplateWrapExpr:
		return i.runtimeCollectionPreservesSourceKeys(
			moduleAddress, expression.Wrapped, budget, depth+1,
		)
	case *hclsyntax.ForExpr:
		return runtimeForExpressionPreservesKeys(
			expression,
			func(collection hcl.Expression, childDepth int) (bool, error) {
				return i.runtimeCollectionPreservesSourceKeys(
					moduleAddress, collection, budget, childDepth,
				)
			},
			depth,
		)
	case *hclsyntax.FunctionCallExpr:
		if expression.Name != "tomap" || len(expression.Args) != 1 ||
			expression.ExpandFinal {
			return false, nil
		}
		return i.runtimeCollectionPreservesSourceKeys(
			moduleAddress, expression.Args[0], budget, depth+1,
		)
	case *hclsyntax.ScopeTraversalExpr:
		traversal, explicitResource :=
			runtimeTraversalWithoutResourcePrefix(
				expression.Traversal,
			)
		if len(traversal) != 2 {
			return false, nil
		}
		root, ok := traversal[0].(hcl.TraverseRoot)
		if !ok {
			return false, nil
		}
		name, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			return false, nil
		}
		key := runtimeExpressionKey{
			moduleAddress: moduleAddress,
			name:          name.Name,
			attribute:     "for_each",
		}
		if root.Name == "module" && !explicitResource {
			key.kind = runtimeExpressionModuleCall
		} else {
			key.kind = runtimeExpressionResource
			key.resourceType = root.Name
		}
		_, declared := i.runtimeIdentityExpressions[key]
		return declared, nil
	default:
		return false, nil
	}
}

func runtimeForExpressionCollection(
	reference string,
	collections map[string]struct{},
) string {
	for start := 0; start < len(reference); {
		separator := strings.IndexByte(reference[start:], '.')
		if separator < 0 {
			break
		}
		separator += start
		if _, ok := collections[reference[:separator]]; ok {
			return reference[:separator]
		}
		start = separator + 1
	}
	if _, ok := collections[reference]; ok {
		return reference
	}
	return ""
}

func visitRuntimeTraversalReferences(
	traversal hcl.Traversal,
	scope map[string][]string,
	visit func(string) error,
) error {
	if len(traversal) == 0 {
		return nil
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return nil
	}
	reference := string(hclwrite.TokensForTraversal(traversal).Bytes())
	suffix, ok := strings.CutPrefix(reference, root.Name)
	if !ok {
		return nil
	}
	if sources, scoped := scope[root.Name]; scoped {
		for _, source := range sources {
			if err := visit(source + suffix); err != nil {
				return err
			}
		}
		return nil
	}
	if normalized, explicitResource :=
		runtimeTraversalWithoutResourcePrefix(traversal); explicitResource {
		reference = string(hclwrite.TokensForTraversal(normalized).Bytes())
	}
	return visit(reference)
}
