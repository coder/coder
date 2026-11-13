package httpmw

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// serviceAccountOrgRolePrefix starts the role every service account holds in
// each of its organizations.
var serviceAccountOrgRolePrefix = rbac.RoleOrgServiceAccount() + ":"

// mayBeDelegatedIdentity reports whether the account these roles belong to
// can be a delegated identity. A delegated identity is a service account in
// one organization, so anyone without that role is spared the lookup.
func mayBeDelegatedIdentity(roles []string) bool {
	for _, role := range roles {
		if strings.HasPrefix(role, serviceAccountOrgRolePrefix) {
			return true
		}
	}
	return false
}

// DelegatedIdentityScopes returns what a delegated identity may do: the scopes
// on its own record that every delegated identity above it holds as well.
// chain is the identity and the identities above it, nearest first. Lowering
// a parent therefore lowers everything below it at once.
func DelegatedIdentityScopes(chain []database.DelegatedIdentity) []rbac.ScopeName {
	if len(chain) == 0 {
		return nil
	}
	scopes := canonicalScopeNames(chain[0].Scopes)
	for _, ancestor := range chain[1:] {
		scopes = CoveredScopes(canonicalScopeNames(ancestor.Scopes), scopes)
	}
	return scopes
}

// CoveredScopes returns the requested scopes that the allowed scopes confer.
// A scope that cannot be compared is not conferred.
func CoveredScopes(allowed, requested []rbac.ScopeName) []rbac.ScopeName {
	kept := make([]rbac.ScopeName, 0, len(requested))
	for _, name := range requested {
		covered, err := rbac.ScopesCover(allowed, name)
		if err != nil || !covered {
			continue
		}
		kept = append(kept, name)
	}
	return kept
}

func canonicalScopeNames(scopes []string) []rbac.ScopeName {
	names := make([]rbac.ScopeName, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, rbac.CanonicalScopeName(rbac.ScopeName(scope)))
	}
	return names
}

// delegatedIdentityScope is the RBAC scope of a delegated identity's
// credential. Unlike an API key, an identity without scopes can do nothing.
type delegatedIdentityScope struct {
	identityID uuid.UUID
	scopes     database.APIKeyScopes
	allowList  database.AllowList
}

func (s delegatedIdentityScope) Name() rbac.RoleIdentifier {
	names := make([]string, 0, len(s.scopes))
	for _, scope := range s.scopes {
		names = append(names, string(scope))
	}
	return rbac.RoleIdentifier{Name: "delegated-identity[" + s.identityID.String() + "]" + strings.Join(names, "+")}
}

func (s delegatedIdentityScope) Expand() (rbac.Scope, error) {
	if len(s.scopes) == 0 {
		return rbac.Scope{
			Role:        rbac.Role{Identifier: s.Name()},
			AllowIDList: []rbac.AllowListElement{rbac.AllowListAll()},
		}, nil
	}
	return s.scopes.WithAllowList(s.allowList).Expand()
}

// apiKeyRBACSubject builds the subject of an API key. The key of a delegated
// identity is worth what the identity may do, read now, and no more than the
// key itself was given.
func apiKeyRBACSubject(ctx context.Context, db database.Store, key database.APIKey) (rbac.Subject, database.UserStatus, []database.DelegatedIdentity, error) {
	return subjectWithDelegation(ctx, db, key.UserID, func(chain []database.DelegatedIdentity) (rbac.ExpandableScope, error) {
		if len(chain) == 0 {
			return key.ScopeSet(), nil
		}
		effective := DelegatedIdentityScopes(chain)
		// A key without scopes of its own predates scopes and has them all.
		if len(key.Scopes) > 0 {
			keyScopes := make([]string, 0, len(key.Scopes))
			for _, scope := range key.Scopes {
				keyScopes = append(keyScopes, string(scope))
			}
			effective = CoveredScopes(canonicalScopeNames(keyScopes), effective)
		}
		scopes := make(database.APIKeyScopes, 0, len(effective))
		for _, scope := range effective {
			scopes = append(scopes, database.APIKeyScope(scope))
		}
		return delegatedIdentityScope{
			identityID: chain[0].ID,
			scopes:     scopes,
			allowList:  key.ScopeSet().AllowList,
		}, nil
	})
}

// DelegatedIdentityRBACSubject builds the subject a delegated identity acts
// as when Coder acts for it without one of its credentials, for example in a
// chat that runs under the identity. It is what a credential of the identity
// would be worth: its own ID, the roles and groups of its root user, and no
// more than its scopes allow. It is an error to ask for an account that is
// not a delegated identity.
func DelegatedIdentityRBACSubject(ctx context.Context, db database.Store, identityID uuid.UUID) (rbac.Subject, database.UserStatus, error) {
	subject, status, _, err := subjectWithDelegation(ctx, db, identityID, func(chain []database.DelegatedIdentity) (rbac.ExpandableScope, error) {
		if len(chain) == 0 {
			return nil, xerrors.Errorf("account %s is not a delegated identity", identityID)
		}
		effective := DelegatedIdentityScopes(chain)
		scopes := make(database.APIKeyScopes, 0, len(effective))
		for _, scope := range effective {
			scopes = append(scopes, database.APIKeyScope(scope))
		}
		return delegatedIdentityScope{
			identityID: chain[0].ID,
			scopes:     scopes,
			allowList:  database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
		}, nil
	})
	return subject, status, err
}
