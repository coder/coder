// Package capabilities resolves the coarse-grained capabilities a user holds,
// derived from the RBAC engine's verdict on representative actions.
package capabilities

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/quartz"
)

// Capability names a coarse-grained ability a user holds.
type Capability string

// Workspace is held by users the RBAC engine authorizes to create a workspace
// they own, in any organization.
const Workspace Capability = "workspace"

// Strings returns the capabilities as a sorted, deduplicated string slice.
func Strings(caps []Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Checker resolves the capabilities held by a user.
type Checker interface {
	// Capabilities returns the capabilities the user currently holds. The
	// returned slice may be empty.
	Capabilities(ctx context.Context, userID uuid.UUID) ([]Capability, error)
}

// Noop resolves no capabilities.
type Noop struct{}

var _ Checker = Noop{}

func (Noop) Capabilities(context.Context, uuid.UUID) ([]Capability, error) {
	return nil, nil
}

// DefaultCacheTTL bounds how long a resolved capability set is reused. Role and
// group changes are not observed until the entry expires.
const DefaultCacheTTL = 5 * time.Minute

// defaultCacheMaxEntries bounds memory use. The cache is cleared wholesale once
// it exceeds this size rather than evicting in access order.
const defaultCacheMaxEntries = 4096

type cacheEntry struct {
	caps      []Capability
	expiresAt time.Time
}

// DBChecker resolves capabilities by expanding a user's stored roles and asking
// the RBAC engine. Results are cached per user for the configured TTL.
type DBChecker struct {
	db         database.Store
	authorizer rbac.Authorizer
	logger     slog.Logger
	clock      quartz.Clock
	ttl        time.Duration

	mu    sync.Mutex
	cache map[uuid.UUID]cacheEntry
}

var _ Checker = (*DBChecker)(nil)

// Options configures a DBChecker. Clock and CacheTTL are optional.
type Options struct {
	DB         database.Store
	Authorizer rbac.Authorizer
	Logger     slog.Logger
	Clock      quartz.Clock
	CacheTTL   time.Duration
}

func NewDBChecker(opts Options) (*DBChecker, error) {
	if opts.DB == nil {
		return nil, xerrors.New("database is required")
	}
	if opts.Authorizer == nil {
		return nil, xerrors.New("authorizer is required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &DBChecker{
		db:         opts.DB,
		authorizer: opts.Authorizer,
		logger:     opts.Logger,
		clock:      clock,
		ttl:        ttl,
		cache:      make(map[uuid.UUID]cacheEntry),
	}, nil
}

func (c *DBChecker) Capabilities(ctx context.Context, userID uuid.UUID) ([]Capability, error) {
	if caps, ok := c.cached(userID); ok {
		return caps, nil
	}

	caps, err := c.resolve(ctx, userID)
	if err != nil {
		return nil, err
	}

	c.store(userID, caps)
	return caps, nil
}

func (c *DBChecker) cached(userID uuid.UUID) ([]Capability, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[userID]
	if !ok || !entry.expiresAt.After(c.clock.Now()) {
		return nil, false
	}
	return slices.Clone(entry.caps), true
}

func (c *DBChecker) store(userID uuid.UUID, caps []Capability) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= defaultCacheMaxEntries {
		c.cache = make(map[uuid.UUID]cacheEntry)
	}
	c.cache[userID] = cacheEntry{
		caps:      slices.Clone(caps),
		expiresAt: c.clock.Now().Add(c.ttl),
	}
}

func (c *DBChecker) resolve(ctx context.Context, userID uuid.UUID) ([]Capability, error) {
	// Reading another user's roles and expanding custom roles are both system
	// operations; the caller's own authorization context cannot perform them.
	//nolint:gocritic // Resolving capabilities is a system function.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	row, err := c.db.GetAuthorizationUserRoles(sysCtx, userID)
	if err != nil {
		return nil, xerrors.Errorf("get authorization user roles: %w", err)
	}
	// Roles are ignored during authorization for users who are not active, so
	// they hold no capabilities.
	if row.Status != database.UserStatusActive {
		return nil, nil
	}

	roleNames, err := row.RoleNames()
	if err != nil {
		// Authorization fails closed on an unparsable role string, so the user
		// holds no capabilities.
		c.logger.Warn(ctx, "user has an unparsable role, resolving no capabilities",
			slog.F("user_id", userID),
			slog.Error(err),
		)
		return nil, nil
	}

	subject := subjectFor(userID, roleNames, row.Groups)
	roles, err := rolestore.Expand(sysCtx, c.db, subject.SafeRoleNames())
	if err != nil {
		return nil, xerrors.Errorf("expand roles: %w", err)
	}
	subject.Roles = roles
	subject = subject.WithCachedASTValue()

	var caps []Capability
	capable, err := c.canCreateWorkspace(ctx, subject)
	if err != nil {
		return nil, xerrors.Errorf("authorize workspace create: %w", err)
	}
	if capable {
		caps = append(caps, Workspace)
	}
	return caps, nil
}

// canCreateWorkspace reports whether the RBAC engine authorizes the subject to
// create a workspace they own in any organization: via membership grants or via
// a site-wide role that applies regardless of membership.
//
// The any-organization form allows exactly when some per-organization check
// would, because the policy takes the maximum vote across the subject's
// memberships, and it also covers users who belong to zero organizations.
//
// Only an authorization denial reports false. Any other failure, such as a
// canceled context or an evaluation error, is returned as an error so the
// caller does not record it as a denial.
func (c *DBChecker) canCreateWorkspace(ctx context.Context, subject rbac.Subject) (bool, error) {
	err := c.authorizer.Authorize(ctx, subject, policy.ActionCreate,
		rbac.ResourceWorkspace.AnyOrganization().WithOwner(subject.ID))
	switch {
	case err == nil:
		return true, nil
	case rbac.IsUnauthorizedError(err):
		return false, nil
	default:
		return false, err
	}
}

// subjectFor builds the evaluation subject for a user, with roles and groups
// sorted and deduplicated.
func subjectFor(userID uuid.UUID, roleNames []rbac.RoleIdentifier, groups []string) rbac.Subject {
	roleNames = slices.Clone(roleNames)
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
		ID:     userID.String(),
		Roles:  rbac.RoleIdentifiers(roleNames),
		Groups: groups,
		Scope:  rbac.ScopeAll,
	}
}
