package xjson

import (
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// ParseUUIDList parses a JSON-encoded array of UUID strings
// (e.g. `["uuid1","uuid2"]`) and returns the corresponding
// slice of uuid.UUID values. An empty or blank input returns
// an empty (non-nil) slice.
func ParseUUIDList(raw string) ([]uuid.UUID, error) {
	if raw == "" {
		return []uuid.UUID{}, nil
	}

	var strings []string
	if err := json.Unmarshal([]byte(raw), &strings); err != nil {
		return nil, xerrors.Errorf("unmarshal uuid list: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(strings))
	for _, s := range strings {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, xerrors.Errorf("parse uuid %q: %w", s, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
