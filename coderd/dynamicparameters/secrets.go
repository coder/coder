package dynamicparameters

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	previewtypes "github.com/coder/preview/types"
)

// EvaluateSecretMismatch reports whether the given template version
// declares coder_secret requirements that the workspace owner's secrets
// do not satisfy. Returns false (no mismatch) when the renderer cannot
// authoritatively evaluate the requirements; the reason is logged at the
// appropriate level so operators can distinguish a forbidden caller
// (expected for template admins) from a genuine renderer or DB failure.
// Returns ErrTemplateVersionNotReady when the version's provisioner job
// has not yet completed; callers should treat that as "unknown" and
// leave SecretMismatch false.
func EvaluateSecretMismatch(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	cache files.FileAcquirer,
	version database.TemplateVersion,
	ownerID uuid.UUID,
	buildParams []database.WorkspaceBuildParameter,
) (bool, error) {
	paramValues := slice.ToMapFunc(buildParams, func(p database.WorkspaceBuildParameter) (string, string) {
		return p.Name, p.Value
	})
	renderer, err := Prepare(ctx, db, cache, version.ID,
		WithTemplateVersion(version),
		WithLogger(logger))
	if err != nil {
		return false, err
	}
	defer renderer.Close()

	result, diags := renderer.Render(ctx, ownerID, paramValues, IncludeSecretRequirements())

	// Three distinct "unknown" cases. Returning false from any of them
	// matches the resolve-autostart handler's semantics, but they have
	// very different operator implications, so we log accordingly. The
	// renderer already logs its own diagnostics through the same logger,
	// so we omit them here to avoid duplication.
	if result.Output == nil {
		logger.Warn(ctx,
			"secret requirement evaluation produced no preview output; treating as unknown",
			slog.F("template_version_id", version.ID),
		)
		return false, nil
	}
	switch secretValidationBlockerCode(diags) {
	case DiagCodeOwnerSecretsFetchFailed:
		logger.Warn(ctx,
			"failed to fetch owner secrets during requirement evaluation; treating as unknown",
			slog.F("template_version_id", version.ID),
		)
		return false, nil
	case DiagCodeSecretValidationForbidden:
		// Expected when a caller without user_secret:read on the owner
		// hits the renderer, e.g. a template admin viewing another user's
		// workspace. Debug-level keeps production volume sane while
		// preserving visibility under trace logging.
		logger.Debug(ctx,
			"secret requirement evaluation forbidden for caller; treating as unknown",
			slog.F("template_version_id", version.ID),
		)
		return false, nil
	}

	return slices.ContainsFunc(result.SecretRequirements,
		func(s codersdk.SecretRequirementStatus) bool { return !s.Satisfied }), nil
}

// secretValidationBlockerCode returns the first diagnostic code among the
// codes that indicate secret-requirement evaluation could not be
// performed. Returns the empty string if no such diagnostic is present.
//
// ExtractDiagnosticExtra walks the wrapped-extra chain so we still
// detect our marker when another extra has been chained on top by
// preview's SetDiagnosticExtra.
func secretValidationBlockerCode(diags hcl.Diagnostics) string {
	for _, d := range diags {
		extra := previewtypes.ExtractDiagnosticExtra(d)
		switch extra.Code {
		case DiagCodeOwnerSecretsFetchFailed, DiagCodeSecretValidationForbidden:
			return extra.Code
		}
	}
	return ""
}
