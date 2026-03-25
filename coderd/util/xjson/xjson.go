package xjson

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// ParseUUIDList parses a JSON-encoded array of UUID strings
// (e.g. `["uuid1","uuid2"]`) and returns the corresponding
// slice of uuid.UUID values. An empty input (including
// whitespace-only) returns an empty (non-nil) slice.
func ParseUUIDList(raw string) ([]uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []uuid.UUID{}, nil
	}

	var strs []string
	if err := json.Unmarshal([]byte(raw), &strs); err != nil {
		return nil, xerrors.Errorf("unmarshal uuid list: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(strs))
	for _, s := range strs {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, xerrors.Errorf("parse uuid %q: %w", s, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
