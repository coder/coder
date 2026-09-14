package prebuilds

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// FindMatchingPresetID finds a preset ID that matches the provided parameters.
// It returns the preset ID if a match is found, or uuid.Nil if no match is found.
// The function performs a bidirectional comparison to ensure all parameters match exactly.
func FindMatchingPresetID(
	ctx context.Context,
	store database.Store,
	templateVersionID uuid.UUID,
	parameterNames []string,
	parameterValues []string,
) (uuid.UUID, error) {
	if len(parameterNames) != len(parameterValues) {
		return uuid.Nil, xerrors.New("parameter names and values must have the same length")
	}

	result, err := store.FindMatchingPresetID(ctx, database.FindMatchingPresetIDParams{
		TemplateVersionID: templateVersionID,
		ParameterNames:    parameterNames,
		ParameterValues:   parameterValues,
	})
	if err != nil {
		// Handle the case where no matching preset is found (no rows returned)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, xerrors.Errorf("find matching preset ID: %w", err)
	}

	return result, nil
}
