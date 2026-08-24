package license

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

// countingSubjectID replaces the real user ID in every evaluated subject
// and on the object owner. The substitution is safe because the policy
// only ever compares the subject ID to the object owner, and the
// evaluated object is synthetic with no user or group ACL lists, so no
// other rule can reference a real ID. Subjects with equal roles and
// groups are therefore byte-identical.
var countingSubjectID = uuid.MustParse("ad966897-b805-4a2c-8dab-3cfcbba0a683").String()

// CountWorkspaceCapableUsers returns the number of active users the RBAC
// engine authorizes to create a workspace, either in one of the
// organizations they belong to or in any organization via a site-wide
// role such as owner. System users and service accounts are excluded by
// the underlying query, matching GetActiveUserCount.
func CountWorkspaceCapableUsers(ctx context.Context, logger slog.Logger, db database.Store, authorizer rbac.Authorizer) (int64, error) {
	if authorizer == nil {
		return 0, xerrors.New("dev error: authorizer is required")
	}

	start := time.Now()

	// All custom roles are prefetched into the context's role cache in a
	// single query; role expansion below then resolves both builtin and
	// custom roles without per-role-set database lookups.
	//nolint:gocritic // Counting licensed seats is a system function.
	ctx, err := rolestore.PrefetchCustomRoles(dbauthz.AsSystemRestricted(ctx), db)
	if err != nil {
		return 0, xerrors.Errorf("prefetch custom roles: %w", err)
	}

	//nolint:gocritic // Counting licensed seats is a system function.
	rows, err := db.GetActiveUsersAuthorizationRoles(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return 0, xerrors.Errorf("get active users authorization roles: %w", err)
	}

	// Users with equivalent canonical subjects share one authorization
	// verdict, so evaluation cost scales with unique subjects, not users.
	capableBySignature := make(map[[sha256.Size]byte]bool)
	var count int64
	for _, row := range rows {
		roleNames, err := row.RoleNames()
		if err != nil {
			// A stored role string that fails to parse grants nothing:
			// authorization fails closed on it, so this user cannot
			// create a workspace. Treat the user as not capable instead
			// of failing the entire count.
			logger.Warn(ctx, "user has an unparsable role, counting them as not workspace-capable for license seats",
				slog.F("user_id", row.ID),
				slog.Error(err),
			)
			continue
		}
		subject := countingSubject(roleNames, row.Groups)
		sig, err := authorizationSignature(subject)
		if err != nil {
			return 0, xerrors.Errorf("compute authorization signature for user %s: %w", row.ID, err)
		}
		capable, ok := capableBySignature[sig]
		if !ok {
			capable, err = canCreateWorkspace(ctx, db, authorizer, subject)
			if err != nil {
				return 0, xerrors.Errorf("evaluate workspace-create for user %s: %w", row.ID, err)
			}
			capableBySignature[sig] = capable
		}
		if capable {
			count++
		}
	}

	// Emitted only when workspace-capable counting runs, so the line's
	// presence identifies the counting mode (workspace-capable vs. all
	// active users).
	logger.Info(ctx, "counted workspace-capable users for license seats",
		slog.F("workspace_capable_users", count),
		slog.F("active_users", len(rows)),
		slog.F("unique_subjects", len(capableBySignature)),
		slog.F("elapsed", time.Since(start)),
	)
	return count, nil
}

// countingSubject builds the canonical evaluation subject for a user.
func countingSubject(roleNames []rbac.RoleIdentifier, groups []string) rbac.Subject {
	slices.SortFunc(roleNames, func(a, b rbac.RoleIdentifier) int {
		return strings.Compare(a.String(), b.String())
	})
	roleNames = slices.CompactFunc(roleNames, func(a, b rbac.RoleIdentifier) bool {
		return a == b
	})
	groups = slices.Clone(groups)
	slices.Sort(groups)
	groups = slices.Compact(groups)
	return rbac.Subject{
		Type:   rbac.SubjectTypeUser,
		ID:     countingSubjectID,
		Roles:  rbac.RoleIdentifiers(roleNames),
		Groups: groups,
		Scope:  rbac.ScopeAll,
	}
}

// authorizationSignature returns a hash of the subject's JSON form;
// every subject field, including any added later, is part of the key.
func authorizationSignature(subject rbac.Subject) ([sha256.Size]byte, error) {
	var sig [sha256.Size]byte
	hash := sha256.New()
	if err := json.NewEncoder(hash).Encode(subject); err != nil {
		return sig, xerrors.Errorf("encode subject: %w", err)
	}
	copy(sig[:], hash.Sum(nil))
	return sig, nil
}

// canCreateWorkspace reports whether the RBAC engine authorizes the
// subject to create a workspace they own in any organization: via
// membership grants or via a site-wide role that applies regardless of
// membership.
func canCreateWorkspace(ctx context.Context, db database.Store, authorizer rbac.Authorizer, subject rbac.Subject) (bool, error) {
	//nolint:gocritic // Expanding custom roles requires system access.
	roles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, subject.SafeRoleNames())
	if err != nil {
		return false, xerrors.Errorf("expand roles: %w", err)
	}
	subject.Roles = roles
	subject = subject.WithCachedASTValue()

	// The any-organization form allows exactly when some per-organization
	// check would (the policy takes the maximum vote across the subject's
	// memberships), and also covers users who belong to zero organizations.
	return authorizer.Authorize(ctx, subject, policy.ActionCreate,
		rbac.ResourceWorkspace.AnyOrganization().WithOwner(subject.ID)) == nil, nil
}
