package database

import (
	"errors"

	"github.com/lib/pq"
)

// UniqueConstraint represents a named unique constraint on a table.
type UniqueConstraint string

// UniqueConstraint enums.
// TODO(mafredri): Generate these from the database schema.
const (
	UniqueConstraintAny                       UniqueConstraint = ""
	UniqueConstraintWorkspacesOwnerIDLowerIdx UniqueConstraint = "workspaces_owner_id_lower_idx"
)

func IsUniqueViolation(err error, uniqueConstraint UniqueConstraint) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code.Name() == "unique_violation" {
			if pqErr.Constraint == string(uniqueConstraint) || uniqueConstraint == UniqueConstraintAny {
				return true
			}
		}
	}

	return false
}
