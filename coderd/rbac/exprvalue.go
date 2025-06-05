package rbac

import (
	"github.com/coder/coder/v2/coderd/expr"
	"github.com/google/uuid"
)

// ExprSubject returns the expr.Subject representation of the subject.
func (s Subject) ExprSubject() expr.Subject {
	exprRoles := make([]expr.Role, 0, len(s.SafeRoleNames()))
	for _, role := range s.SafeRoleNames() {
		// Convert zero UUID (site-wide roles) to empty string for easier expr expression writing
		orgID := role.OrganizationID.String()
		if orgID == uuid.Nil.String() {
			orgID = ""
		}

		exprRoles = append(exprRoles, expr.Role{
			Name:  role.Name,
			OrgID: orgID,
		})
	}

	return expr.Subject{
		ID:     s.ID,
		Email:  s.Email,
		Groups: s.Groups,
		Roles:  exprRoles,
	}
}
