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
	UniqueWorkspacesOwnerIDLowerIdx UniqueConstraint = "workspaces_owner_id_lower_idx"
)

// IsUniqueViolation checks if the error is due to a unique violation.
// If one or more specific unique constraints are given as arguments,
// the error must be caused by one of them. If no constraints are given,
// this function returns true for any unique violation.
func IsUniqueViolation(err error, uniqueConstraints ...UniqueConstraint) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code.Name() == "unique_violation" {
			if len(uniqueConstraints) == 0 {
				return true
			}
			for _, uc := range uniqueConstraints {
				if pqErr.Constraint == string(uc) {
					return true
				}
			}
		}
	}

	return false
}
