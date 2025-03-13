package uuid

import (
	"strings"

	"github.com/google/uuid"
)

func FromSliceToString(uuids []uuid.UUID, separator string) string {
	uuidStrings := make([]string, 0, len(uuids))

	for _, u := range uuids {
		uuidStrings = append(uuidStrings, u.String())
	}

	return strings.Join(uuidStrings, separator)
}
