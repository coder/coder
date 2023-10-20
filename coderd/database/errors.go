package database

import (
	"context"
	"errors"

	"github.com/lib/pq"
)

func IsSerializedError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code.Name() == "serialization_failure"
	}
	return false
}

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

// IsForeignKeyViolation checks if the error is due to a foreign key violation.
// If one or more specific foreign key constraints are given as arguments,
// the error must be caused by one of them. If no constraints are given,
// this function returns true for any foreign key violation.
func IsForeignKeyViolation(err error, foreignKeyConstraints ...ForeignKeyConstraint) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code.Name() == "foreign_key_violation" {
			if len(foreignKeyConstraints) == 0 {
				return true
			}
			for _, fc := range foreignKeyConstraints {
				if pqErr.Constraint == string(fc) {
					return true
				}
			}
		}
	}

	return false
}

// IsQueryCanceledError checks if the error is due to a query being canceled.
func IsQueryCanceledError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "57014" // query_canceled
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}

func IsWorkspaceAgentLogsLimitError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Constraint == "max_logs_length" && pqErr.Table == "workspace_agents"
	}

	return false
}
