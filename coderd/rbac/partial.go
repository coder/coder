package rbac

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

type partialAuthorizer struct {
	// PartialRego is mainly used for unit testing. It is the rego source policy.
	PartialRego   *rego.Rego
	PartialResult rego.PartialResult
	Input         map[string]interface{}
}

func newPartialAuthorizer(ctx context.Context, partialRego *rego.Rego, input map[string]interface{}) (*partialAuthorizer, error) {
	pResult, err := partialRego.PartialResult(ctx)
	if err != nil {
		return nil, xerrors.Errorf("partial results: %w", err)
	}

	return &partialAuthorizer{
		PartialRego:   partialRego,
		PartialResult: pResult,
		Input:         input,
	}, nil
}

// Authorize authorizes a single object
func (a partialAuthorizer) Authorize(ctx context.Context, object Object) error {
	results, err := a.PartialResult.Rego(rego.Input(
		map[string]interface{}{
			"object": object,
		})).Eval(ctx)
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("eval prepared"), a.Input, results)
	}

	if !results.Allowed() {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), a.Input, results)
	}

	return nil
}
