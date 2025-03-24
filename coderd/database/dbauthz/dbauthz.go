package dbauthz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/topdown"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi/httpapiconstraints"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/provisionersdk"
)

var _ database.Store = (*querier)(nil)

const wrapname = "dbauthz.querier"

// NoActorError is returned if no actor is present in the context.
var NoActorError = xerrors.Errorf("no authorization actor in context")

// NotAuthorizedError is a sentinel error that unwraps to sql.ErrNoRows.
// This allows the internal error to be read by the caller if needed. Otherwise
// it will be handled as a 404.
type NotAuthorizedError struct {
	Err error
}

// Ensure we implement the IsUnauthorized interface.
var _ httpapiconstraints.IsUnauthorizedError = (*NotAuthorizedError)(nil)

func (e NotAuthorizedError) Error() string {
	var detail string
	if e.Err != nil {
		detail = ": " + e.Err.Error()
	}
	return "unauthorized" + detail
}

// IsUnauthorized implements the IsUnauthorized interface.
func (NotAuthorizedError) IsUnauthorized() bool {
	return true
}

// Unwrap will always unwrap to a sql.ErrNoRows so the API returns a 404.
// So 'errors.Is(err, sql.ErrNoRows)' will always be true.
func (e NotAuthorizedError) Unwrap() error {
	return e.Err
}

func IsNotAuthorizedError(err error) bool {
	if err == nil {
		return false
	}
	if xerrors.Is(err, NoActorError) {
		return true
	}

	return xerrors.As(err, &NotAuthorizedError{})
}

func logNotAuthorizedError(ctx context.Context, logger slog.Logger, err error) error {
	// Only log the errors if it is an UnauthorizedError error.
	internalError := new(rbac.UnauthorizedError)
	if err != nil && xerrors.As(err, &internalError) {
		e := new(topdown.Error)
		if xerrors.As(err, &e) || e.Code == topdown.CancelErr {
			// For some reason rego changes a canceled context to a topdown.CancelErr. We
			// expect to check for canceled context errors if the user cancels the request,
			// so we should change the error to a context.Canceled error.
			//
			// NotAuthorizedError is == to sql.ErrNoRows, which is not correct
			// if it's actually a canceled context.
			contextError := *internalError
			contextError.SetInternal(context.Canceled)
			return &contextError
		}
		logger.Debug(ctx, "unauthorized",
			slog.F("internal_error", internalError.Internal()),
			slog.F("input", internalError.Input()),
			slog.Error(err),
		)
	}

	return NotAuthorizedError{
		Err: err,
	}
}

// querier is a wrapper around the database store that performs authorization
// checks before returning data. All querier methods expect an authorization
// subject present in the context. If no subject is present, most methods will
// fail.
//
// Use WithAuthorizeContext to set the authorization subject in the context for
// the common user case.
type querier struct {
	db   database.Store
	auth rbac.Authorizer
	log  slog.Logger
	acs  *atomic.Pointer[AccessControlStore]
}

func New(db database.Store, authorizer rbac.Authorizer, logger slog.Logger, acs *atomic.Pointer[AccessControlStore]) database.Store {
	// If the underlying db store is already a querier, return it.
	// Do not double wrap.
	if slices.Contains(db.Wrappers(), wrapname) {
		return db
	}
	return &querier{
		db:   db,
		auth: authorizer,
		log:  logger,
		acs:  acs,
	}
}

func (q *querier) Wrappers() []string {
	return append(q.db.Wrappers(), wrapname)
}

// authorizeContext is a helper function to authorize an action on an object.
func (q *querier) authorizeContext(ctx context.Context, action policy.Action, object rbac.Objecter) error {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	err := q.auth.Authorize(ctx, act, action, object.RBACObject())
	if err != nil {
		return logNotAuthorizedError(ctx, q.log, err)
	}
	return nil
}

type authContextKey struct{}

// ActorFromContext returns the authorization subject from the context.
// All authentication flows should set the authorization subject in the context.
// If no actor is present, the function returns false.
func ActorFromContext(ctx context.Context) (rbac.Subject, bool) {
	a, ok := ctx.Value(authContextKey{}).(rbac.Subject)
	return a, ok
}

var (
	subjectProvisionerd = rbac.Subject{
		FriendlyName: "Provisioner Daemon",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "provisionerd"},
				DisplayName: "Provisioner Daemon",
				Site: rbac.Permissions(map[string][]policy.Action{
					// TODO: Add ProvisionerJob resource type.
					rbac.ResourceFile.Type:     {policy.ActionRead},
					rbac.ResourceSystem.Type:   {policy.WildcardSymbol},
					rbac.ResourceTemplate.Type: {policy.ActionRead, policy.ActionUpdate},
					// Unsure why provisionerd needs update and read personal
					rbac.ResourceUser.Type:             {policy.ActionRead, policy.ActionReadPersonal, policy.ActionUpdatePersonal},
					rbac.ResourceWorkspaceDormant.Type: {policy.ActionDelete, policy.ActionRead, policy.ActionUpdate, policy.ActionWorkspaceStop},
					rbac.ResourceWorkspace.Type:        {policy.ActionDelete, policy.ActionRead, policy.ActionUpdate, policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
					rbac.ResourceApiKey.Type:           {policy.WildcardSymbol},
					// When org scoped provisioner credentials are implemented,
					// this can be reduced to read a specific org.
					rbac.ResourceOrganization.Type: {policy.ActionRead},
					rbac.ResourceGroup.Type:        {policy.ActionRead},
					// Provisionerd creates notification messages
					rbac.ResourceNotificationMessage.Type: {policy.ActionCreate, policy.ActionRead},
					// Provisionerd creates workspaces resources monitor
					rbac.ResourceWorkspaceAgentResourceMonitor.Type: {policy.ActionCreate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectAutostart = rbac.Subject{
		FriendlyName: "Autostart",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "autostart"},
				DisplayName: "Autostart Daemon",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceNotificationMessage.Type: {policy.ActionCreate, policy.ActionRead},
					rbac.ResourceSystem.Type:              {policy.WildcardSymbol},
					rbac.ResourceTemplate.Type:            {policy.ActionRead, policy.ActionUpdate},
					rbac.ResourceUser.Type:                {policy.ActionRead},
					rbac.ResourceWorkspace.Type:           {policy.ActionDelete, policy.ActionRead, policy.ActionUpdate, policy.ActionWorkspaceStart, policy.ActionWorkspaceStop},
					rbac.ResourceWorkspaceDormant.Type:    {policy.ActionDelete, policy.ActionRead, policy.ActionUpdate, policy.ActionWorkspaceStop},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	// See unhanger package.
	subjectHangDetector = rbac.Subject{
		FriendlyName: "Hang Detector",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "hangdetector"},
				DisplayName: "Hang Detector Daemon",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceSystem.Type:    {policy.WildcardSymbol},
					rbac.ResourceTemplate.Type:  {policy.ActionRead},
					rbac.ResourceWorkspace.Type: {policy.ActionRead, policy.ActionUpdate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	// See cryptokeys package.
	subjectCryptoKeyRotator = rbac.Subject{
		FriendlyName: "Crypto Key Rotator",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "keyrotator"},
				DisplayName: "Key Rotator",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceCryptoKey.Type: {policy.WildcardSymbol},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	// See cryptokeys package.
	subjectCryptoKeyReader = rbac.Subject{
		FriendlyName: "Crypto Key Reader",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "keyrotator"},
				DisplayName: "Key Rotator",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceCryptoKey.Type: {policy.WildcardSymbol},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectNotifier = rbac.Subject{
		FriendlyName: "Notifier",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "notifier"},
				DisplayName: "Notifier",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceNotificationMessage.Type: {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceInboxNotification.Type:   {policy.ActionCreate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectResourceMonitor = rbac.Subject{
		FriendlyName: "Resource Monitor",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "resourcemonitor"},
				DisplayName: "Resource Monitor",
				Site: rbac.Permissions(map[string][]policy.Action{
					// The workspace monitor needs to be able to update monitors
					rbac.ResourceWorkspaceAgentResourceMonitor.Type: {policy.ActionUpdate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectSystemRestricted = rbac.Subject{
		FriendlyName: "System",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "system"},
				DisplayName: "Coder",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceWildcard.Type:               {policy.ActionRead},
					rbac.ResourceApiKey.Type:                 rbac.ResourceApiKey.AvailableActions(),
					rbac.ResourceGroup.Type:                  {policy.ActionCreate, policy.ActionUpdate},
					rbac.ResourceAssignRole.Type:             rbac.ResourceAssignRole.AvailableActions(),
					rbac.ResourceAssignOrgRole.Type:          rbac.ResourceAssignOrgRole.AvailableActions(),
					rbac.ResourceSystem.Type:                 {policy.WildcardSymbol},
					rbac.ResourceOrganization.Type:           {policy.ActionCreate, policy.ActionRead},
					rbac.ResourceOrganizationMember.Type:     {policy.ActionCreate, policy.ActionDelete, policy.ActionRead},
					rbac.ResourceProvisionerDaemon.Type:      {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate},
					rbac.ResourceUser.Type:                   rbac.ResourceUser.AvailableActions(),
					rbac.ResourceWorkspaceDormant.Type:       {policy.ActionUpdate, policy.ActionDelete, policy.ActionWorkspaceStop},
					rbac.ResourceWorkspace.Type:              {policy.ActionUpdate, policy.ActionDelete, policy.ActionWorkspaceStart, policy.ActionWorkspaceStop, policy.ActionSSH},
					rbac.ResourceWorkspaceProxy.Type:         {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceDeploymentConfig.Type:       {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceNotificationMessage.Type:    {policy.ActionCreate, policy.ActionRead, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceNotificationPreference.Type: {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceNotificationTemplate.Type:   {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
					rbac.ResourceCryptoKey.Type:              {policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectSystemReadProvisionerDaemons = rbac.Subject{
		FriendlyName: "Provisioner Daemons Reader",
		ID:           uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "system-read-provisioner-daemons"},
				DisplayName: "Coder",
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceProvisionerDaemon.Type: {policy.ActionRead},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectPrebuildsOrchestrator = rbac.Subject{
		FriendlyName: "Prebuilds Orchestrator",
		ID:           prebuilds.SystemUserID.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Identifier:  rbac.RoleIdentifier{Name: "prebuilds-orchestrator"},
				DisplayName: "Coder",
				Site: rbac.Permissions(map[string][]policy.Action{
					// May use template, read template-related info, & insert template-related resources (preset prebuilds).
					rbac.ResourceTemplate.Type: {policy.ActionRead, policy.ActionUpdate, policy.ActionUse},
					// May CRUD workspaces, and start/stop them.
					rbac.ResourceWorkspace.Type: {
						policy.ActionCreate, policy.ActionDelete, policy.ActionRead, policy.ActionUpdate,
						policy.ActionWorkspaceStart, policy.ActionWorkspaceStop,
					},
				}),
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()
)

// AsProvisionerd returns a context with an actor that has permissions required
// for provisionerd to function.
func AsProvisionerd(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectProvisionerd)
}

// AsAutostart returns a context with an actor that has permissions required
// for autostart to function.
func AsAutostart(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectAutostart)
}

// AsHangDetector returns a context with an actor that has permissions required
// for unhanger.Detector to function.
func AsHangDetector(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectHangDetector)
}

// AsKeyRotator returns a context with an actor that has permissions required for rotating crypto keys.
func AsKeyRotator(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectCryptoKeyRotator)
}

// AsKeyReader returns a context with an actor that has permissions required for reading crypto keys.
func AsKeyReader(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectCryptoKeyReader)
}

// AsNotifier returns a context with an actor that has permissions required for
// creating/reading/updating/deleting notifications.
func AsNotifier(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectNotifier)
}

// AsResourceMonitor returns a context with an actor that has permissions required for
// updating resource monitors.
func AsResourceMonitor(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectResourceMonitor)
}

// AsSystemRestricted returns a context with an actor that has permissions
// required for various system operations (login, logout, metrics cache).
func AsSystemRestricted(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectSystemRestricted)
}

// AsSystemReadProvisionerDaemons returns a context with an actor that has permissions
// to read provisioner daemons.
func AsSystemReadProvisionerDaemons(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectSystemReadProvisionerDaemons)
}

// AsPrebuildsOrchestrator returns a context with an actor that has permissions
// to read orchestrator workspace prebuilds.
func AsPrebuildsOrchestrator(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectPrebuildsOrchestrator)
}

var AsRemoveActor = rbac.Subject{
	ID: "remove-actor",
}

// As returns a context with the given actor stored in the context.
// This is used for cases where the actor touching the database is not the
// actor stored in the context.
// When you use this function, be sure to add a //nolint comment
// explaining why it is necessary.
func As(ctx context.Context, actor rbac.Subject) context.Context {
	if actor.Equal(AsRemoveActor) {
		// AsRemoveActor is a special case that is used to indicate that the actor
		// should be removed from the context.
		return context.WithValue(ctx, authContextKey{}, nil)
	}
	return context.WithValue(ctx, authContextKey{}, actor)
}

//
// Generic functions used to implement the database.Store methods.
//

// insert runs an policy.ActionCreate on the rbac object argument before
// running the insertFunc. The insertFunc is expected to return the object that
// was inserted.
func insert[
	ObjectType any,
	ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	object rbac.Objecter,
	insertFunc Insert,
) Insert {
	return insertWithAction(logger, authorizer, object, policy.ActionCreate, insertFunc)
}

func insertWithAction[
	ObjectType any,
	ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	object rbac.Objecter,
	action policy.Action,
	insertFunc Insert,
) Insert {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
		if err != nil {
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		// Insert the database object
		return insertFunc(ctx, arg)
	}
}

func deleteQ[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete,
) Delete {
	return fetchAndExec(logger, authorizer,
		policy.ActionDelete, fetchFunc, deleteFunc)
}

func updateWithReturn[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	UpdateQuery func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateQuery UpdateQuery,
) UpdateQuery {
	return fetchAndQuery(logger, authorizer, policy.ActionUpdate, fetchFunc, updateQuery)
}

func update[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateExec Exec,
) Exec {
	return fetchAndExec(logger, authorizer, policy.ActionUpdate, fetchFunc, updateExec)
}

// fetch is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
func fetchWithAction[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action policy.Action,
	f DatabaseFunc,
) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Fetch the database object
		object, err := f(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
		if err != nil {
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		return object, nil
	}
}

func fetch[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	f DatabaseFunc,
) DatabaseFunc {
	return fetchWithAction(logger, authorizer, policy.ActionRead, f)
}

// fetchAndExec uses fetchAndQuery but only returns the error. The naming comes
// from SQL 'exec' functions which only return an error.
// See fetchAndQuery for more information.
func fetchAndExec[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action policy.Action,
	fetchFunc Fetch,
	execFunc Exec,
) Exec {
	f := fetchAndQuery(logger, authorizer, action, fetchFunc, func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		return empty, execFunc(ctx, arg)
	})
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := f(ctx, arg)
		return err
	}
}

// fetchAndQuery is a generic function that wraps a database fetch and query.
// A query has potential side effects in the database (update, delete, etc).
// The fetch is used to know which rbac object the action should be asserted on
// **before** the query runs. The returns from the fetch are only used to
// assert rbac. The final return of this function comes from the Query function.
func fetchAndQuery[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Query func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action policy.Action,
	fetchFunc Fetch,
	queryFunc Query,
) Query {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Fetch the database object
		object, err := fetchFunc(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
		if err != nil {
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		return queryFunc(ctx, arg)
	}
}

// fetchWithPostFilter is like fetch, but works with lists of objects.
// SQL filters are much more optimal.
func fetchWithPostFilter[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) ([]ObjectType, error),
](
	authorizer rbac.Authorizer,
	action policy.Action,
	f DatabaseFunc,
) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty []ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Fetch the database object
		objects, err := f(ctx, arg)
		if err != nil {
			return nil, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		return rbac.Filter(ctx, authorizer, act, action, objects)
	}
}

// prepareSQLFilter is a helper function that prepares a SQL filter using the
// given authorization context.
func prepareSQLFilter(ctx context.Context, authorizer rbac.Authorizer, action policy.Action, resourceType string) (rbac.PreparedAuthorized, error) {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return nil, NoActorError
	}

	return authorizer.Prepare(ctx, act, action, resourceType)
}

func (q *querier) Ping(ctx context.Context) (time.Duration, error) {
	return q.db.Ping(ctx)
}

func (q *querier) PGLocks(ctx context.Context) (database.PGLocks, error) {
	return q.db.PGLocks(ctx)
}

// InTx runs the given function in a transaction.
func (q *querier) InTx(function func(querier database.Store) error, txOpts *database.TxOptions) error {
	return q.db.InTx(func(tx database.Store) error {
		// Wrap the transaction store in a querier.
		wrapped := New(tx, q.auth, q.log, q.acs)
		return function(wrapped)
	}, txOpts)
}

// authorizeReadFile is a hotfix for the fact that file permissions are
// independent of template permissions. This function checks if the user has
// update access to any of the file's templates.
func (q *querier) authorizeUpdateFileTemplate(ctx context.Context, file database.File) error {
	tpls, err := q.db.GetFileTemplates(ctx, file.ID)
	if err != nil {
		return err
	}
	// There __should__ only be 1 template per file, but there can be more than
	// 1, so check them all.
	for _, tpl := range tpls {
		// If the user has update access to any template, they have read access to the file.
		if err := q.authorizeContext(ctx, policy.ActionUpdate, tpl); err == nil {
			return nil
		}
	}

	return NotAuthorizedError{
		Err: xerrors.Errorf("not authorized to read file %s", file.ID),
	}
}

// convertToOrganizationRoles converts a set of scoped role names to their unique
// scoped names. The database stores roles as an array of strings, and needs to be
// converted.
// TODO: Maybe make `[]rbac.RoleIdentifier` a custom type that implements a sql scanner
// to remove the need for these converters?
func (*querier) convertToOrganizationRoles(organizationID uuid.UUID, names []string) ([]rbac.RoleIdentifier, error) {
	uniques := make([]rbac.RoleIdentifier, 0, len(names))
	for _, name := range names {
		// This check is a developer safety check. Old code might try to invoke this code path with
		// organization id suffixes. Catch this and return a nice error so it can be fixed.
		if strings.Contains(name, ":") {
			return nil, xerrors.Errorf("attempt to assign a role %q, remove the ':<organization_id> suffix", name)
		}

		uniques = append(uniques, rbac.RoleIdentifier{Name: name, OrganizationID: organizationID})
	}

	return uniques, nil
}

// convertToDeploymentRoles converts string role names into deployment wide roles.
func (*querier) convertToDeploymentRoles(names []string) []rbac.RoleIdentifier {
	uniques := make([]rbac.RoleIdentifier, 0, len(names))
	for _, name := range names {
		uniques = append(uniques, rbac.RoleIdentifier{Name: name})
	}

	return uniques
}

// canAssignRoles handles assigning built in and custom roles.
func (q *querier) canAssignRoles(ctx context.Context, orgID uuid.UUID, added, removed []rbac.RoleIdentifier) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	roleAssign := rbac.ResourceAssignRole
	shouldBeOrgRoles := false
	if orgID != uuid.Nil {
		roleAssign = rbac.ResourceAssignOrgRole.InOrg(orgID)
		shouldBeOrgRoles = true
	}

	grantedRoles := make([]rbac.RoleIdentifier, 0, len(added)+len(removed))
	grantedRoles = append(grantedRoles, added...)
	grantedRoles = append(grantedRoles, removed...)
	customRoles := make([]rbac.RoleIdentifier, 0)
	// Validate that the roles being assigned are valid.
	for _, r := range grantedRoles {
		isOrgRole := r.OrganizationID != uuid.Nil
		if shouldBeOrgRoles && !isOrgRole {
			return xerrors.Errorf("Must only update org roles")
		}

		if !shouldBeOrgRoles && isOrgRole {
			return xerrors.Errorf("Must only update site wide roles")
		}

		if shouldBeOrgRoles {
			if orgID == uuid.Nil {
				return xerrors.Errorf("should never happen, orgID is nil, but trying to assign an organization role")
			}

			if r.OrganizationID != orgID {
				return xerrors.Errorf("attempted to assign role from a different org, role %q to %q", r, orgID.String())
			}
		}

		// All roles should be valid roles
		if _, err := rbac.RoleByName(r); err != nil {
			customRoles = append(customRoles, r)
		}
	}

	customRolesMap := make(map[rbac.RoleIdentifier]struct{}, len(customRoles))
	for _, r := range customRoles {
		customRolesMap[r] = struct{}{}
	}

	if len(customRoles) > 0 {
		// Leverage any custom role cache that might exist.
		expandedCustomRoles, err := rolestore.Expand(ctx, q.db, customRoles)
		if err != nil {
			return xerrors.Errorf("fetching custom roles: %w", err)
		}

		// If the lists are not identical, then have a problem, as some roles
		// provided do no exist.
		if len(customRoles) != len(expandedCustomRoles) {
			for _, role := range customRoles {
				// Stop at the first one found. We could make a better error that
				// returns them all, but then someone could pass in a large list to make us do
				// a lot of loop iterations.
				if !slices.ContainsFunc(expandedCustomRoles, func(customRole rbac.Role) bool {
					return strings.EqualFold(customRole.Identifier.Name, role.Name) && customRole.Identifier.OrganizationID == role.OrganizationID
				}) {
					return xerrors.Errorf("%q is not a supported role", role)
				}
			}
		}
	}

	if len(added) > 0 {
		if err := q.authorizeContext(ctx, policy.ActionAssign, roleAssign); err != nil {
			return err
		}
	}

	if len(removed) > 0 {
		if err := q.authorizeContext(ctx, policy.ActionUnassign, roleAssign); err != nil {
			return err
		}
	}

	for _, roleName := range grantedRoles {
		if _, isCustom := customRolesMap[roleName]; isCustom {
			// To support a dynamic mapping of what roles can assign what, we need
			// to store this in the database. For now, just use a static role so
			// owners and org admins can assign roles.
			if roleName.IsOrgRole() {
				roleName = rbac.CustomOrganizationRole(roleName.OrganizationID)
			} else {
				roleName = rbac.CustomSiteRole()
			}
		}

		if !rbac.CanAssignRole(actor.Roles, roleName) {
			return xerrors.Errorf("not authorized to assign role %q", roleName)
		}
	}

	return nil
}

func (q *querier) SoftDeleteTemplateByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:        id,
			Deleted:   true,
			UpdatedAt: dbtime.Now(),
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetTemplateByID, deleteF)(ctx, id)
}

func (q *querier) SoftDeleteWorkspaceByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetWorkspaceByID, func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	})(ctx, id)
}

func authorizedTemplateVersionFromJob(ctx context.Context, q *querier, job database.ProvisionerJob) (database.TemplateVersion, error) {
	switch job.Type {
	case database.ProvisionerJobTypeTemplateVersionDryRun:
		// TODO: This is really unfortunate that we need to inspect the json
		// payload. We should fix this.
		tmp := struct {
			TemplateVersionID uuid.UUID `json:"template_version_id"`
		}{}
		err := json.Unmarshal(job.Input, &tmp)
		if err != nil {
			return database.TemplateVersion{}, xerrors.Errorf("dry-run unmarshal: %w", err)
		}
		// Authorized call to get template version.
		tv, err := q.GetTemplateVersionByID(ctx, tmp.TemplateVersionID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		return tv, nil
	case database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		tv, err := q.GetTemplateVersionByJobID(ctx, job.ID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		return tv, nil
	default:
		return database.TemplateVersion{}, xerrors.Errorf("unknown job type: %q", job.Type)
	}
}

func (q *querier) authorizeTemplateInsights(ctx context.Context, templateIDs []uuid.UUID) error {
	// Abort early if can read all template insights, aka admins.
	// TODO: If we know the org, that would allow org admins to abort early too.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		for _, templateID := range templateIDs {
			template, err := q.db.GetTemplateByID(ctx, templateID)
			if err != nil {
				return err
			}

			if err := q.authorizeContext(ctx, policy.ActionViewInsights, template); err != nil {
				return err
			}
		}
		if len(templateIDs) == 0 {
			if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate.All()); err != nil {
				return err
			}
		}
	}
	return nil
}

// customRoleEscalationCheck checks to make sure the caller has every permission they are adding
// to a custom role. This prevents permission escalation.
func (q *querier) customRoleEscalationCheck(ctx context.Context, actor rbac.Subject, perm rbac.Permission, object rbac.Object) error {
	if perm.Negate {
		// Users do not need negative permissions. We can include it later if required.
		return xerrors.Errorf("invalid permission for action=%q type=%q, no negative permissions", perm.Action, perm.ResourceType)
	}

	if perm.Action == policy.WildcardSymbol || perm.ResourceType == policy.WildcardSymbol {
		// It is possible to check for supersets with wildcards, but wildcards can also
		// include resources and actions that do not exist today. Custom roles should only be allowed
		// to include permissions for existing resources.
		return xerrors.Errorf("invalid permission for action=%q type=%q, no wildcard symbols", perm.Action, perm.ResourceType)
	}

	object.Type = perm.ResourceType
	if err := q.auth.Authorize(ctx, actor, perm.Action, object); err != nil {
		// This is a forbidden error, but we can provide more context. Since the user can create a role, just not
		// with this perm.
		return xerrors.Errorf("invalid permission for action=%q type=%q, not allowed to grant this permission", perm.Action, perm.ResourceType)
	}

	return nil
}

// customRoleCheck will validate a custom role for inserting or updating.
// If the role is not valid, an error will be returned.
// - Check custom roles are valid for their resource types + actions
// - Check the actor can create the custom role
// - Check the custom role does not grant perms the actor does not have
// - Prevent negative perms
// - Prevent roles with site and org permissions.
func (q *querier) customRoleCheck(ctx context.Context, role database.CustomRole) error {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	// Org permissions require an org role
	if role.OrganizationID.UUID == uuid.Nil && len(role.OrgPermissions) > 0 {
		return xerrors.Errorf("organization permissions require specifying an organization id")
	}

	// Org roles can only specify org permissions
	if role.OrganizationID.UUID != uuid.Nil && (len(role.SitePermissions) > 0 || len(role.UserPermissions) > 0) {
		return xerrors.Errorf("organization roles specify site or user permissions")
	}

	// The rbac.Role has a 'Valid()' function on it that will do a lot
	// of checks.
	rbacRole, err := rolestore.ConvertDBRole(database.CustomRole{
		Name:            role.Name,
		DisplayName:     role.DisplayName,
		SitePermissions: role.SitePermissions,
		OrgPermissions:  role.OrgPermissions,
		UserPermissions: role.UserPermissions,
		OrganizationID:  role.OrganizationID,
	})
	if err != nil {
		return xerrors.Errorf("invalid args: %w", err)
	}

	err = rbacRole.Valid()
	if err != nil {
		return xerrors.Errorf("invalid role: %w", err)
	}

	if len(rbacRole.Org) > 0 && len(rbacRole.Site) > 0 {
		// This is a choice to keep roles simple. If we allow mixing site and org scoped perms, then knowing who can
		// do what gets more complicated.
		return xerrors.Errorf("invalid custom role, cannot assign both org and site permissions at the same time")
	}

	if len(rbacRole.Org) > 1 {
		// Again to avoid more complexity in our roles
		return xerrors.Errorf("invalid custom role, cannot assign permissions to more than 1 org at a time")
	}

	// Prevent escalation
	for _, sitePerm := range rbacRole.Site {
		err := q.customRoleEscalationCheck(ctx, act, sitePerm, rbac.Object{Type: sitePerm.ResourceType})
		if err != nil {
			return xerrors.Errorf("site permission: %w", err)
		}
	}

	for orgID, perms := range rbacRole.Org {
		for _, orgPerm := range perms {
			err := q.customRoleEscalationCheck(ctx, act, orgPerm, rbac.Object{OrgID: orgID, Type: orgPerm.ResourceType})
			if err != nil {
				return xerrors.Errorf("org=%q: %w", orgID, err)
			}
		}
	}

	for _, userPerm := range rbacRole.User {
		err := q.customRoleEscalationCheck(ctx, act, userPerm, rbac.Object{Type: userPerm.ResourceType, Owner: act.ID})
		if err != nil {
			return xerrors.Errorf("user permission: %w", err)
		}
	}

	return nil
}

func (q *querier) AcquireLock(ctx context.Context, id int64) error {
	return q.db.AcquireLock(ctx, id)
}

func (q *querier) AcquireNotificationMessages(ctx context.Context, arg database.AcquireNotificationMessagesParams) ([]database.AcquireNotificationMessagesRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceNotificationMessage); err != nil {
		return nil, err
	}
	return q.db.AcquireNotificationMessages(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.AcquireProvisionerJob(ctx, arg)
}

func (q *querier) ActivityBumpWorkspace(ctx context.Context, arg database.ActivityBumpWorkspaceParams) error {
	fetch := func(ctx context.Context, arg database.ActivityBumpWorkspaceParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	}
	return update(q.log, q.auth, fetch, q.db.ActivityBumpWorkspace)(ctx, arg)
}

func (q *querier) AllUserIDs(ctx context.Context, includeSystem bool) ([]uuid.UUID, error) {
	// Although this technically only reads users, only system-related functions should be
	// allowed to call this.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.AllUserIDs(ctx, includeSystem)
}

func (q *querier) ArchiveUnusedTemplateVersions(ctx context.Context, arg database.ArchiveUnusedTemplateVersionsParams) ([]uuid.UUID, error) {
	tpl, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, tpl); err != nil {
		return nil, err
	}
	return q.db.ArchiveUnusedTemplateVersions(ctx, arg)
}

func (q *querier) BatchUpdateWorkspaceLastUsedAt(ctx context.Context, arg database.BatchUpdateWorkspaceLastUsedAtParams) error {
	// Could be any workspace and checking auth to each workspace is overkill for the purpose
	// of this function.
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceWorkspace.All()); err != nil {
		return err
	}
	return q.db.BatchUpdateWorkspaceLastUsedAt(ctx, arg)
}

func (q *querier) BatchUpdateWorkspaceNextStartAt(ctx context.Context, arg database.BatchUpdateWorkspaceNextStartAtParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceWorkspace.All()); err != nil {
		return err
	}
	return q.db.BatchUpdateWorkspaceNextStartAt(ctx, arg)
}

func (q *querier) BulkMarkNotificationMessagesFailed(ctx context.Context, arg database.BulkMarkNotificationMessagesFailedParams) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceNotificationMessage); err != nil {
		return 0, err
	}
	return q.db.BulkMarkNotificationMessagesFailed(ctx, arg)
}

func (q *querier) BulkMarkNotificationMessagesSent(ctx context.Context, arg database.BulkMarkNotificationMessagesSentParams) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceNotificationMessage); err != nil {
		return 0, err
	}
	return q.db.BulkMarkNotificationMessagesSent(ctx, arg)
}

func (q *querier) ClaimPrebuild(ctx context.Context, newOwnerID database.ClaimPrebuildParams) (database.ClaimPrebuildRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceWorkspace); err != nil {
		return database.ClaimPrebuildRow{
			ID: uuid.Nil,
		}, err
	}
	return q.db.ClaimPrebuild(ctx, newOwnerID)
}

func (q *querier) CleanTailnetCoordinators(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.CleanTailnetCoordinators(ctx)
}

func (q *querier) CleanTailnetLostPeers(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.CleanTailnetLostPeers(ctx)
}

func (q *querier) CleanTailnetTunnels(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.CleanTailnetTunnels(ctx)
}

func (q *querier) CountUnreadInboxNotificationsByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceInboxNotification.WithOwner(userID.String())); err != nil {
		return 0, err
	}
	return q.db.CountUnreadInboxNotificationsByUserID(ctx, userID)
}

// TODO: Handle org scoped lookups
func (q *querier) CustomRoles(ctx context.Context, arg database.CustomRolesParams) ([]database.CustomRole, error) {
	roleObject := rbac.ResourceAssignRole
	if arg.OrganizationID != uuid.Nil {
		roleObject = rbac.ResourceAssignOrgRole.InOrg(arg.OrganizationID)
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, roleObject); err != nil {
		return nil, err
	}

	return q.db.CustomRoles(ctx, arg)
}

func (q *querier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return deleteQ(q.log, q.auth, q.db.GetAPIKeyByID, q.db.DeleteAPIKeyByID)(ctx, id)
}

func (q *querier) DeleteAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	// TODO: This is not 100% correct because it omits apikey IDs.
	err := q.authorizeContext(ctx, policy.ActionDelete,
		rbac.ResourceApiKey.WithOwner(userID.String()))
	if err != nil {
		return err
	}
	return q.db.DeleteAPIKeysByUserID(ctx, userID)
}

func (q *querier) DeleteAllTailnetClientSubscriptions(ctx context.Context, arg database.DeleteAllTailnetClientSubscriptionsParams) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteAllTailnetClientSubscriptions(ctx, arg)
}

func (q *querier) DeleteAllTailnetTunnels(ctx context.Context, arg database.DeleteAllTailnetTunnelsParams) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteAllTailnetTunnels(ctx, arg)
}

func (q *querier) DeleteApplicationConnectAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	// TODO: This is not 100% correct because it omits apikey IDs.
	err := q.authorizeContext(ctx, policy.ActionDelete,
		rbac.ResourceApiKey.WithOwner(userID.String()))
	if err != nil {
		return err
	}
	return q.db.DeleteApplicationConnectAPIKeysByUserID(ctx, userID)
}

func (q *querier) DeleteCoordinator(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteCoordinator(ctx, id)
}

func (q *querier) DeleteCryptoKey(ctx context.Context, arg database.DeleteCryptoKeyParams) (database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceCryptoKey); err != nil {
		return database.CryptoKey{}, err
	}
	return q.db.DeleteCryptoKey(ctx, arg)
}

func (q *querier) DeleteCustomRole(ctx context.Context, arg database.DeleteCustomRoleParams) error {
	if !arg.OrganizationID.Valid || arg.OrganizationID.UUID == uuid.Nil {
		return NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")}
	}
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceAssignOrgRole.InOrg(arg.OrganizationID.UUID)); err != nil {
		return err
	}

	return q.db.DeleteCustomRole(ctx, arg)
}

func (q *querier) DeleteExternalAuthLink(ctx context.Context, arg database.DeleteExternalAuthLinkParams) error {
	return fetchAndExec(q.log, q.auth, policy.ActionUpdatePersonal, func(ctx context.Context, arg database.DeleteExternalAuthLinkParams) (database.ExternalAuthLink, error) {
		//nolint:gosimple
		return q.db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{UserID: arg.UserID, ProviderID: arg.ProviderID})
	}, q.db.DeleteExternalAuthLink)(ctx, arg)
}

func (q *querier) DeleteGitSSHKey(ctx context.Context, userID uuid.UUID) error {
	return fetchAndExec(q.log, q.auth, policy.ActionUpdatePersonal, q.db.GetGitSSHKey, q.db.DeleteGitSSHKey)(ctx, userID)
}

func (q *querier) DeleteGroupByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetGroupByID, q.db.DeleteGroupByID)(ctx, id)
}

func (q *querier) DeleteGroupMemberFromGroup(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) error {
	// Deleting a group member counts as updating a group.
	fetch := func(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.GroupID)
	}
	return update(q.log, q.auth, fetch, q.db.DeleteGroupMemberFromGroup)(ctx, arg)
}

func (q *querier) DeleteLicense(ctx context.Context, id int32) (int32, error) {
	err := deleteQ(q.log, q.auth, q.db.GetLicenseByID, func(ctx context.Context, id int32) error {
		_, err := q.db.DeleteLicense(ctx, id)
		return err
	})(ctx, id)
	if err != nil {
		return -1, err
	}
	return id, nil
}

func (q *querier) DeleteOAuth2ProviderAppByID(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceOauth2App); err != nil {
		return err
	}
	return q.db.DeleteOAuth2ProviderAppByID(ctx, id)
}

func (q *querier) DeleteOAuth2ProviderAppCodeByID(ctx context.Context, id uuid.UUID) error {
	code, err := q.db.GetOAuth2ProviderAppCodeByID(ctx, id)
	if err != nil {
		return err
	}
	if err := q.authorizeContext(ctx, policy.ActionDelete, code); err != nil {
		return err
	}
	return q.db.DeleteOAuth2ProviderAppCodeByID(ctx, id)
}

func (q *querier) DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx context.Context, arg database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete,
		rbac.ResourceOauth2AppCodeToken.WithOwner(arg.UserID.String())); err != nil {
		return err
	}
	return q.db.DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx, arg)
}

func (q *querier) DeleteOAuth2ProviderAppSecretByID(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceOauth2AppSecret); err != nil {
		return err
	}
	return q.db.DeleteOAuth2ProviderAppSecretByID(ctx, id)
}

func (q *querier) DeleteOAuth2ProviderAppTokensByAppAndUserID(ctx context.Context, arg database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete,
		rbac.ResourceOauth2AppCodeToken.WithOwner(arg.UserID.String())); err != nil {
		return err
	}
	return q.db.DeleteOAuth2ProviderAppTokensByAppAndUserID(ctx, arg)
}

func (q *querier) DeleteOldNotificationMessages(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceNotificationMessage); err != nil {
		return err
	}
	return q.db.DeleteOldNotificationMessages(ctx)
}

func (q *querier) DeleteOldProvisionerDaemons(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldProvisionerDaemons(ctx)
}

func (q *querier) DeleteOldWorkspaceAgentLogs(ctx context.Context, threshold time.Time) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentLogs(ctx, threshold)
}

func (q *querier) DeleteOldWorkspaceAgentStats(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentStats(ctx)
}

func (q *querier) DeleteOrganizationMember(ctx context.Context, arg database.DeleteOrganizationMemberParams) error {
	return deleteQ[database.OrganizationMember](q.log, q.auth, func(ctx context.Context, arg database.DeleteOrganizationMemberParams) (database.OrganizationMember, error) {
		member, err := database.ExpectOne(q.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: arg.OrganizationID,
			UserID:         arg.UserID,
			IncludeSystem:  false,
		}))
		if err != nil {
			return database.OrganizationMember{}, err
		}
		return member.OrganizationMember, nil
	}, q.db.DeleteOrganizationMember)(ctx, arg)
}

func (q *querier) DeleteProvisionerKey(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetProvisionerKeyByID, q.db.DeleteProvisionerKey)(ctx, id)
}

func (q *querier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *querier) DeleteRuntimeConfig(ctx context.Context, key string) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteRuntimeConfig(ctx, key)
}

func (q *querier) DeleteTailnetAgent(ctx context.Context, arg database.DeleteTailnetAgentParams) (database.DeleteTailnetAgentRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetAgentRow{}, err
	}
	return q.db.DeleteTailnetAgent(ctx, arg)
}

func (q *querier) DeleteTailnetClient(ctx context.Context, arg database.DeleteTailnetClientParams) (database.DeleteTailnetClientRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetClientRow{}, err
	}
	return q.db.DeleteTailnetClient(ctx, arg)
}

func (q *querier) DeleteTailnetClientSubscription(ctx context.Context, arg database.DeleteTailnetClientSubscriptionParams) error {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteTailnetClientSubscription(ctx, arg)
}

func (q *querier) DeleteTailnetPeer(ctx context.Context, arg database.DeleteTailnetPeerParams) (database.DeleteTailnetPeerRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetPeerRow{}, err
	}
	return q.db.DeleteTailnetPeer(ctx, arg)
}

func (q *querier) DeleteTailnetTunnel(ctx context.Context, arg database.DeleteTailnetTunnelParams) (database.DeleteTailnetTunnelRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetTunnelRow{}, err
	}
	return q.db.DeleteTailnetTunnel(ctx, arg)
}

func (q *querier) DeleteWorkspaceAgentPortShare(ctx context.Context, arg database.DeleteWorkspaceAgentPortShareParams) error {
	w, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return err
	}

	// deleting a workspace port share is more akin to just updating the workspace.
	if err = q.authorizeContext(ctx, policy.ActionUpdate, w.RBACObject()); err != nil {
		return xerrors.Errorf("authorize context: %w", err)
	}

	return q.db.DeleteWorkspaceAgentPortShare(ctx, arg)
}

func (q *querier) DeleteWorkspaceAgentPortSharesByTemplate(ctx context.Context, templateID uuid.UUID) error {
	template, err := q.db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return err
	}

	return q.db.DeleteWorkspaceAgentPortSharesByTemplate(ctx, templateID)
}

func (q *querier) DisableForeignKeysAndTriggers(ctx context.Context) error {
	if !testing.Testing() {
		return xerrors.Errorf("DisableForeignKeysAndTriggers is only allowed in tests")
	}
	return q.db.DisableForeignKeysAndTriggers(ctx)
}

func (q *querier) EnqueueNotificationMessage(ctx context.Context, arg database.EnqueueNotificationMessageParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceNotificationMessage); err != nil {
		return err
	}
	return q.db.EnqueueNotificationMessage(ctx, arg)
}

func (q *querier) FavoriteWorkspace(ctx context.Context, id uuid.UUID) error {
	fetch := func(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, id)
	}
	return update(q.log, q.auth, fetch, q.db.FavoriteWorkspace)(ctx, id)
}

func (q *querier) FetchMemoryResourceMonitorsByAgentID(ctx context.Context, agentID uuid.UUID) (database.WorkspaceAgentMemoryResourceMonitor, error) {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil {
		return database.WorkspaceAgentMemoryResourceMonitor{}, err
	}

	err = q.authorizeContext(ctx, policy.ActionRead, workspace)
	if err != nil {
		return database.WorkspaceAgentMemoryResourceMonitor{}, err
	}

	return q.db.FetchMemoryResourceMonitorsByAgentID(ctx, agentID)
}

func (q *querier) FetchMemoryResourceMonitorsUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.WorkspaceAgentMemoryResourceMonitor, error) {
	// Ideally, we would return a list of monitors that the user has access to. However, that check would need to
	// be implemented similarly to GetWorkspaces, which is more complex than what we're doing here. Since this query
	// was introduced for telemetry, we perform a simpler check.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return nil, err
	}

	return q.db.FetchMemoryResourceMonitorsUpdatedAfter(ctx, updatedAt)
}

func (q *querier) FetchNewMessageMetadata(ctx context.Context, arg database.FetchNewMessageMetadataParams) (database.FetchNewMessageMetadataRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceNotificationMessage); err != nil {
		return database.FetchNewMessageMetadataRow{}, err
	}
	return q.db.FetchNewMessageMetadata(ctx, arg)
}

func (q *querier) FetchVolumesResourceMonitorsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceAgentVolumeResourceMonitor, error) {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	err = q.authorizeContext(ctx, policy.ActionRead, workspace)
	if err != nil {
		return nil, err
	}

	return q.db.FetchVolumesResourceMonitorsByAgentID(ctx, agentID)
}

func (q *querier) FetchVolumesResourceMonitorsUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.WorkspaceAgentVolumeResourceMonitor, error) {
	// Ideally, we would return a list of monitors that the user has access to. However, that check would need to
	// be implemented similarly to GetWorkspaces, which is more complex than what we're doing here. Since this query
	// was introduced for telemetry, we perform a simpler check.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return nil, err
	}

	return q.db.FetchVolumesResourceMonitorsUpdatedAfter(ctx, updatedAt)
}

func (q *querier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByID)(ctx, id)
}

func (q *querier) GetAPIKeyByName(ctx context.Context, arg database.GetAPIKeyByNameParams) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByName)(ctx, arg)
}

func (q *querier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *querier) GetAPIKeysByUserID(ctx context.Context, params database.GetAPIKeysByUserIDParams) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetAPIKeysByUserID)(ctx, database.GetAPIKeysByUserIDParams{LoginType: params.LoginType, UserID: params.UserID})
}

func (q *querier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *querier) GetActiveUserCount(ctx context.Context, includeSystem bool) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetActiveUserCount(ctx, includeSystem)
}

func (q *querier) GetActiveWorkspaceBuildsByTemplateID(ctx context.Context, templateID uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This is a system-only function.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return []database.WorkspaceBuild{}, err
	}
	return q.db.GetActiveWorkspaceBuildsByTemplateID(ctx, templateID)
}

func (q *querier) GetAllTailnetAgents(ctx context.Context) ([]database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return []database.TailnetAgent{}, err
	}
	return q.db.GetAllTailnetAgents(ctx)
}

func (q *querier) GetAllTailnetCoordinators(ctx context.Context) ([]database.TailnetCoordinator, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetAllTailnetCoordinators(ctx)
}

func (q *querier) GetAllTailnetPeers(ctx context.Context) ([]database.TailnetPeer, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetAllTailnetPeers(ctx)
}

func (q *querier) GetAllTailnetTunnels(ctx context.Context) ([]database.TailnetTunnel, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetAllTailnetTunnels(ctx)
}

func (q *querier) GetAnnouncementBanners(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetAnnouncementBanners(ctx)
}

func (q *querier) GetAppSecurityKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetAppSecurityKey(ctx)
}

func (q *querier) GetApplicationName(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetApplicationName(ctx)
}

func (q *querier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	// Shortcut if the user is an owner. The SQL filter is noticeable,
	// and this is an easy win for owners. Which is the common case.
	err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceAuditLog)
	if err == nil {
		return q.db.GetAuditLogsOffset(ctx, arg)
	}

	prep, err := prepareSQLFilter(ctx, q.auth, policy.ActionRead, rbac.ResourceAuditLog.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}

	return q.db.GetAuthorizedAuditLogsOffset(ctx, arg, prep)
}

func (q *querier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetAuthorizationUserRolesRow{}, err
	}
	return q.db.GetAuthorizationUserRoles(ctx, userID)
}

func (q *querier) GetCoordinatorResumeTokenSigningKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetCoordinatorResumeTokenSigningKey(ctx)
}

func (q *querier) GetCryptoKeyByFeatureAndSequence(ctx context.Context, arg database.GetCryptoKeyByFeatureAndSequenceParams) (database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceCryptoKey); err != nil {
		return database.CryptoKey{}, err
	}
	return q.db.GetCryptoKeyByFeatureAndSequence(ctx, arg)
}

func (q *querier) GetCryptoKeys(ctx context.Context) ([]database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceCryptoKey); err != nil {
		return nil, err
	}
	return q.db.GetCryptoKeys(ctx)
}

func (q *querier) GetCryptoKeysByFeature(ctx context.Context, feature database.CryptoKeyFeature) ([]database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceCryptoKey); err != nil {
		return nil, err
	}
	return q.db.GetCryptoKeysByFeature(ctx, feature)
}

func (q *querier) GetDBCryptKeys(ctx context.Context) ([]database.DBCryptKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetDBCryptKeys(ctx)
}

func (q *querier) GetDERPMeshKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetDERPMeshKey(ctx)
}

func (q *querier) GetDefaultOrganization(ctx context.Context) (database.Organization, error) {
	return fetch(q.log, q.auth, func(ctx context.Context, _ any) (database.Organization, error) {
		return q.db.GetDefaultOrganization(ctx)
	})(ctx, nil)
}

func (q *querier) GetDefaultProxyConfig(ctx context.Context) (database.GetDefaultProxyConfigRow, error) {
	// No authz checks
	return q.db.GetDefaultProxyConfig(ctx)
}

// Only used by metrics cache.
func (q *querier) GetDeploymentDAUs(ctx context.Context, tzOffset int32) ([]database.GetDeploymentDAUsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetDeploymentDAUs(ctx, tzOffset)
}

func (q *querier) GetDeploymentID(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetDeploymentID(ctx)
}

func (q *querier) GetDeploymentWorkspaceAgentStats(ctx context.Context, createdAfter time.Time) (database.GetDeploymentWorkspaceAgentStatsRow, error) {
	return q.db.GetDeploymentWorkspaceAgentStats(ctx, createdAfter)
}

func (q *querier) GetDeploymentWorkspaceAgentUsageStats(ctx context.Context, createdAt time.Time) (database.GetDeploymentWorkspaceAgentUsageStatsRow, error) {
	return q.db.GetDeploymentWorkspaceAgentUsageStats(ctx, createdAt)
}

func (q *querier) GetDeploymentWorkspaceStats(ctx context.Context) (database.GetDeploymentWorkspaceStatsRow, error) {
	return q.db.GetDeploymentWorkspaceStats(ctx)
}

func (q *querier) GetEligibleProvisionerDaemonsByProvisionerJobIDs(ctx context.Context, provisionerJobIds []uuid.UUID) ([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetEligibleProvisionerDaemonsByProvisionerJobIDs)(ctx, provisionerJobIds)
}

func (q *querier) GetExternalAuthLink(ctx context.Context, arg database.GetExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	return fetchWithAction(q.log, q.auth, policy.ActionReadPersonal, q.db.GetExternalAuthLink)(ctx, arg)
}

func (q *querier) GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error) {
	return fetchWithPostFilter(q.auth, policy.ActionReadPersonal, q.db.GetExternalAuthLinksByUserID)(ctx, userID)
}

func (q *querier) GetFailedWorkspaceBuildsByTemplateID(ctx context.Context, arg database.GetFailedWorkspaceBuildsByTemplateIDParams) ([]database.GetFailedWorkspaceBuildsByTemplateIDRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFailedWorkspaceBuildsByTemplateID(ctx, arg)
}

func (q *querier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	file, err := q.db.GetFileByHashAndCreator(ctx, arg)
	if err != nil {
		return database.File{}, err
	}
	err = q.authorizeContext(ctx, policy.ActionRead, file)
	if err != nil {
		// Check the user's access to the file's templates.
		if q.authorizeUpdateFileTemplate(ctx, file) != nil {
			return database.File{}, err
		}
	}

	return file, nil
}

func (q *querier) GetFileByID(ctx context.Context, id uuid.UUID) (database.File, error) {
	file, err := q.db.GetFileByID(ctx, id)
	if err != nil {
		return database.File{}, err
	}
	err = q.authorizeContext(ctx, policy.ActionRead, file)
	if err != nil {
		// Check the user's access to the file's templates.
		if q.authorizeUpdateFileTemplate(ctx, file) != nil {
			return database.File{}, err
		}
	}

	return file, nil
}

func (q *querier) GetFileTemplates(ctx context.Context, fileID uuid.UUID) ([]database.GetFileTemplatesRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFileTemplates(ctx, fileID)
}

func (q *querier) GetFilteredInboxNotificationsByUserID(ctx context.Context, arg database.GetFilteredInboxNotificationsByUserIDParams) ([]database.InboxNotification, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetFilteredInboxNotificationsByUserID)(ctx, arg)
}

func (q *querier) GetGitSSHKey(ctx context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	return fetchWithAction(q.log, q.auth, policy.ActionReadPersonal, q.db.GetGitSSHKey)(ctx, userID)
}

func (q *querier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByID)(ctx, id)
}

func (q *querier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByOrgAndName)(ctx, arg)
}

func (q *querier) GetGroupMembers(ctx context.Context) ([]database.GroupMember, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetGroupMembers(ctx)
}

func (q *querier) GetGroupMembersByGroupID(ctx context.Context, id uuid.UUID) ([]database.GroupMember, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetGroupMembersByGroupID)(ctx, id)
}

func (q *querier) GetGroupMembersCountByGroupID(ctx context.Context, groupID uuid.UUID) (int64, error) {
	if _, err := q.GetGroupByID(ctx, groupID); err != nil { // AuthZ check
		return 0, err
	}
	memberCount, err := q.db.GetGroupMembersCountByGroupID(ctx, groupID)
	if err != nil {
		return 0, err
	}
	return memberCount, nil
}

func (q *querier) GetGroups(ctx context.Context, arg database.GetGroupsParams) ([]database.GetGroupsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err == nil {
		// Optimize this query for system users as it is used in telemetry.
		// Calling authz on all groups in a deployment for telemetry jobs is
		// excessive. Most user calls should have some filtering applied to reduce
		// the size of the set.
		return q.db.GetGroups(ctx, arg)
	}

	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetGroups)(ctx, arg)
}

func (q *querier) GetHealthSettings(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetHealthSettings(ctx)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) GetHungProvisionerJobs(ctx context.Context, hungSince time.Time) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.GetHungProvisionerJobs(ctx, hungSince)
}

func (q *querier) GetInboxNotificationByID(ctx context.Context, id uuid.UUID) (database.InboxNotification, error) {
	return fetchWithAction(q.log, q.auth, policy.ActionRead, q.db.GetInboxNotificationByID)(ctx, id)
}

func (q *querier) GetInboxNotificationsByUserID(ctx context.Context, userID database.GetInboxNotificationsByUserIDParams) ([]database.InboxNotification, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetInboxNotificationsByUserID)(ctx, userID)
}

func (q *querier) GetJFrogXrayScanByWorkspaceAndAgentID(ctx context.Context, arg database.GetJFrogXrayScanByWorkspaceAndAgentIDParams) (database.JfrogXrayScan, error) {
	if _, err := fetch(q.log, q.auth, q.db.GetWorkspaceByID)(ctx, arg.WorkspaceID); err != nil {
		return database.JfrogXrayScan{}, err
	}
	return q.db.GetJFrogXrayScanByWorkspaceAndAgentID(ctx, arg)
}

func (q *querier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetLastUpdateCheck(ctx)
}

func (q *querier) GetLatestCryptoKeyByFeature(ctx context.Context, feature database.CryptoKeyFeature) (database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceCryptoKey); err != nil {
		return database.CryptoKey{}, err
	}
	return q.db.GetLatestCryptoKeyByFeature(ctx, feature)
}

func (q *querier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, workspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
}

func (q *querier) GetLatestWorkspaceBuilds(ctx context.Context) ([]database.WorkspaceBuild, error) {
	// This function is a system function until we implement a join for workspace builds.
	// This is because we need to query for all related workspaces to the returned builds.
	// This is a very inefficient method of fetching the latest workspace builds.
	// We should just join the rbac properties.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetLatestWorkspaceBuilds(ctx)
}

func (q *querier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This function is a system function until we implement a join for workspace builds.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}

	return q.db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, ids)
}

func (q *querier) GetLicenseByID(ctx context.Context, id int32) (database.License, error) {
	return fetch(q.log, q.auth, q.db.GetLicenseByID)(ctx, id)
}

func (q *querier) GetLicenses(ctx context.Context) ([]database.License, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.License, error) {
		return q.db.GetLicenses(ctx)
	}
	return fetchWithPostFilter(q.auth, policy.ActionRead, fetch)(ctx, nil)
}

func (q *querier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetLogoURL(ctx)
}

func (q *querier) GetNotificationMessagesByStatus(ctx context.Context, arg database.GetNotificationMessagesByStatusParams) ([]database.NotificationMessage, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceNotificationMessage); err != nil {
		return nil, err
	}
	return q.db.GetNotificationMessagesByStatus(ctx, arg)
}

func (q *querier) GetNotificationReportGeneratorLogByTemplate(ctx context.Context, arg uuid.UUID) (database.NotificationReportGeneratorLog, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.NotificationReportGeneratorLog{}, err
	}
	return q.db.GetNotificationReportGeneratorLogByTemplate(ctx, arg)
}

func (q *querier) GetNotificationTemplateByID(ctx context.Context, id uuid.UUID) (database.NotificationTemplate, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceNotificationTemplate); err != nil {
		return database.NotificationTemplate{}, err
	}
	return q.db.GetNotificationTemplateByID(ctx, id)
}

func (q *querier) GetNotificationTemplatesByKind(ctx context.Context, kind database.NotificationTemplateKind) ([]database.NotificationTemplate, error) {
	// Anyone can read the system notification templates.
	if kind == database.NotificationTemplateKindSystem {
		return q.db.GetNotificationTemplatesByKind(ctx, kind)
	}

	// TODO(dannyk): handle template ownership when we support user-default notification templates.
	return nil, sql.ErrNoRows
}

func (q *querier) GetNotificationsSettings(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetNotificationsSettings(ctx)
}

func (q *querier) GetOAuth2GithubDefaultEligible(ctx context.Context) (bool, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceDeploymentConfig); err != nil {
		return false, err
	}
	return q.db.GetOAuth2GithubDefaultEligible(ctx)
}

func (q *querier) GetOAuth2ProviderAppByID(ctx context.Context, id uuid.UUID) (database.OAuth2ProviderApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOauth2App); err != nil {
		return database.OAuth2ProviderApp{}, err
	}
	return q.db.GetOAuth2ProviderAppByID(ctx, id)
}

func (q *querier) GetOAuth2ProviderAppCodeByID(ctx context.Context, id uuid.UUID) (database.OAuth2ProviderAppCode, error) {
	return fetch(q.log, q.auth, q.db.GetOAuth2ProviderAppCodeByID)(ctx, id)
}

func (q *querier) GetOAuth2ProviderAppCodeByPrefix(ctx context.Context, secretPrefix []byte) (database.OAuth2ProviderAppCode, error) {
	return fetch(q.log, q.auth, q.db.GetOAuth2ProviderAppCodeByPrefix)(ctx, secretPrefix)
}

func (q *querier) GetOAuth2ProviderAppSecretByID(ctx context.Context, id uuid.UUID) (database.OAuth2ProviderAppSecret, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOauth2AppSecret); err != nil {
		return database.OAuth2ProviderAppSecret{}, err
	}
	return q.db.GetOAuth2ProviderAppSecretByID(ctx, id)
}

func (q *querier) GetOAuth2ProviderAppSecretByPrefix(ctx context.Context, secretPrefix []byte) (database.OAuth2ProviderAppSecret, error) {
	return fetch(q.log, q.auth, q.db.GetOAuth2ProviderAppSecretByPrefix)(ctx, secretPrefix)
}

func (q *querier) GetOAuth2ProviderAppSecretsByAppID(ctx context.Context, appID uuid.UUID) ([]database.OAuth2ProviderAppSecret, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOauth2AppSecret); err != nil {
		return []database.OAuth2ProviderAppSecret{}, err
	}
	return q.db.GetOAuth2ProviderAppSecretsByAppID(ctx, appID)
}

func (q *querier) GetOAuth2ProviderAppTokenByPrefix(ctx context.Context, hashPrefix []byte) (database.OAuth2ProviderAppToken, error) {
	token, err := q.db.GetOAuth2ProviderAppTokenByPrefix(ctx, hashPrefix)
	if err != nil {
		return database.OAuth2ProviderAppToken{}, err
	}
	// The user ID is on the API key so that has to be fetched.
	key, err := q.db.GetAPIKeyByID(ctx, token.APIKeyID)
	if err != nil {
		return database.OAuth2ProviderAppToken{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOauth2AppCodeToken.WithOwner(key.UserID.String())); err != nil {
		return database.OAuth2ProviderAppToken{}, err
	}
	return token, nil
}

func (q *querier) GetOAuth2ProviderApps(ctx context.Context) ([]database.OAuth2ProviderApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOauth2App); err != nil {
		return []database.OAuth2ProviderApp{}, err
	}
	return q.db.GetOAuth2ProviderApps(ctx)
}

func (q *querier) GetOAuth2ProviderAppsByUserID(ctx context.Context, userID uuid.UUID) ([]database.GetOAuth2ProviderAppsByUserIDRow, error) {
	// This authz check is to make sure the caller can read all their own tokens.
	if err := q.authorizeContext(ctx, policy.ActionRead,
		rbac.ResourceOauth2AppCodeToken.WithOwner(userID.String())); err != nil {
		return []database.GetOAuth2ProviderAppsByUserIDRow{}, err
	}
	return q.db.GetOAuth2ProviderAppsByUserID(ctx, userID)
}

func (q *querier) GetOAuthSigningKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetOAuthSigningKey(ctx)
}

func (q *querier) GetOrganizationByID(ctx context.Context, id uuid.UUID) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByID)(ctx, id)
}

func (q *querier) GetOrganizationByName(ctx context.Context, name database.GetOrganizationByNameParams) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByName)(ctx, name)
}

func (q *querier) GetOrganizationIDsByMemberIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	// TODO: This should be rewritten to return a list of database.OrganizationMember for consistent RBAC objects.
	// Currently this row returns a list of org ids per user, which is challenging to check against the RBAC system.
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetOrganizationIDsByMemberIDs)(ctx, ids)
}

func (q *querier) GetOrganizations(ctx context.Context, args database.GetOrganizationsParams) ([]database.Organization, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.Organization, error) {
		return q.db.GetOrganizations(ctx, args)
	}
	return fetchWithPostFilter(q.auth, policy.ActionRead, fetch)(ctx, nil)
}

func (q *querier) GetOrganizationsByUserID(ctx context.Context, userID database.GetOrganizationsByUserIDParams) ([]database.Organization, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetOrganizationsByUserID)(ctx, userID)
}

func (q *querier) GetParameterSchemasByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	version, err := q.db.GetTemplateVersionByJobID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	object := version.RBACObjectNoTemplate()
	if version.TemplateID.Valid {
		tpl, err := q.db.GetTemplateByID(ctx, version.TemplateID.UUID)
		if err != nil {
			return nil, err
		}
		object = version.RBACObject(tpl)
	}

	err = q.authorizeContext(ctx, policy.ActionRead, object)
	if err != nil {
		return nil, err
	}
	return q.db.GetParameterSchemasByJobID(ctx, jobID)
}

func (q *querier) GetPrebuildMetrics(ctx context.Context) ([]database.GetPrebuildMetricsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetPrebuildMetrics(ctx)
}

func (q *querier) GetPrebuildsInProgress(ctx context.Context) ([]database.GetPrebuildsInProgressRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetPrebuildsInProgress(ctx)
}

func (q *querier) GetPresetByWorkspaceBuildID(ctx context.Context, workspaceID uuid.UUID) (database.TemplateVersionPreset, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return database.TemplateVersionPreset{}, err
	}
	return q.db.GetPresetByWorkspaceBuildID(ctx, workspaceID)
}

func (q *querier) GetPresetParametersByTemplateVersionID(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionPresetParameter, error) {
	// An actor can read template version presets if they can read the related template version.
	_, err := q.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	return q.db.GetPresetParametersByTemplateVersionID(ctx, templateVersionID)
}

func (q *querier) GetPresetsBackoff(ctx context.Context, lookback time.Time) ([]database.GetPresetsBackoffRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetPresetsBackoff(ctx, lookback)
}

func (q *querier) GetPresetsByTemplateVersionID(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionPreset, error) {
	// An actor can read template version presets if they can read the related template version.
	_, err := q.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	return q.db.GetPresetsByTemplateVersionID(ctx, templateVersionID)
}

func (q *querier) GetPreviousTemplateVersion(ctx context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	// An actor can read the previous template version if they can read the related template.
	// If no linked template exists, we check if the actor can read *a* template.
	if !arg.TemplateID.Valid {
		if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate.InOrg(arg.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	}
	if _, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID); err != nil {
		return database.TemplateVersion{}, err
	}
	return q.db.GetPreviousTemplateVersion(ctx, arg)
}

func (q *querier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.ProvisionerDaemon, error) {
		return q.db.GetProvisionerDaemons(ctx)
	}
	return fetchWithPostFilter(q.auth, policy.ActionRead, fetch)(ctx, nil)
}

func (q *querier) GetProvisionerDaemonsByOrganization(ctx context.Context, organizationID database.GetProvisionerDaemonsByOrganizationParams) ([]database.ProvisionerDaemon, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetProvisionerDaemonsByOrganization)(ctx, organizationID)
}

func (q *querier) GetProvisionerDaemonsWithStatusByOrganization(ctx context.Context, arg database.GetProvisionerDaemonsWithStatusByOrganizationParams) ([]database.GetProvisionerDaemonsWithStatusByOrganizationRow, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetProvisionerDaemonsWithStatusByOrganization)(ctx, arg)
}

func (q *querier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	job, err := q.db.GetProvisionerJobByID(ctx, id)
	if err != nil {
		return database.ProvisionerJob{}, err
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		// Authorized call to get workspace build. If we can read the build, we
		// can read the job.
		_, err := q.GetWorkspaceBuildByJobID(ctx, id)
		if err != nil {
			return database.ProvisionerJob{}, xerrors.Errorf("fetch related workspace build: %w", err)
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		_, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return database.ProvisionerJob{}, xerrors.Errorf("fetch related template version: %w", err)
		}
	default:
		return database.ProvisionerJob{}, xerrors.Errorf("unknown job type: %q", job.Type)
	}

	return job, nil
}

func (q *querier) GetProvisionerJobTimingsByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ProvisionerJobTiming, error) {
	_, err := q.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return q.db.GetProvisionerJobTimingsByJobID(ctx, jobID)
}

// TODO: We have a ProvisionerJobs resource, but it hasn't been checked for this use-case.
func (q *querier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
	// 	return nil, err
	// }
	return q.db.GetProvisionerJobsByIDs(ctx, ids)
}

// TODO: We have a ProvisionerJobs resource, but it hasn't been checked for this use-case.
func (q *querier) GetProvisionerJobsByIDsWithQueuePosition(ctx context.Context, ids []uuid.UUID) ([]database.GetProvisionerJobsByIDsWithQueuePositionRow, error) {
	return q.db.GetProvisionerJobsByIDsWithQueuePosition(ctx, ids)
}

func (q *querier) GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner(ctx context.Context, arg database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams) ([]database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerRow, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner)(ctx, arg)
}

// TODO: We have a ProvisionerJobs resource, but it hasn't been checked for this use-case.
func (q *querier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.GetProvisionerJobsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetProvisionerKeyByHashedSecret(ctx context.Context, hashedSecret []byte) (database.ProvisionerKey, error) {
	return fetch(q.log, q.auth, q.db.GetProvisionerKeyByHashedSecret)(ctx, hashedSecret)
}

func (q *querier) GetProvisionerKeyByID(ctx context.Context, id uuid.UUID) (database.ProvisionerKey, error) {
	return fetch(q.log, q.auth, q.db.GetProvisionerKeyByID)(ctx, id)
}

func (q *querier) GetProvisionerKeyByName(ctx context.Context, name database.GetProvisionerKeyByNameParams) (database.ProvisionerKey, error) {
	return fetch(q.log, q.auth, q.db.GetProvisionerKeyByName)(ctx, name)
}

func (q *querier) GetProvisionerLogsAfterID(ctx context.Context, arg database.GetProvisionerLogsAfterIDParams) ([]database.ProvisionerJobLog, error) {
	// Authorized read on job lets the actor also read the logs.
	_, err := q.GetProvisionerJobByID(ctx, arg.JobID)
	if err != nil {
		return nil, err
	}
	return q.db.GetProvisionerLogsAfterID(ctx, arg)
}

func (q *querier) GetQuotaAllowanceForUser(ctx context.Context, params database.GetQuotaAllowanceForUserParams) (int64, error) {
	err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceUserObject(params.UserID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaAllowanceForUser(ctx, params)
}

func (q *querier) GetQuotaConsumedForUser(ctx context.Context, params database.GetQuotaConsumedForUserParams) (int64, error) {
	err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceUserObject(params.OwnerID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaConsumedForUser(ctx, params)
}

func (q *querier) GetReplicaByID(ctx context.Context, id uuid.UUID) (database.Replica, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.GetReplicaByID(ctx, id)
}

func (q *querier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetReplicasUpdatedAfter(ctx, updatedAt)
}

func (q *querier) GetRunningPrebuilds(ctx context.Context) ([]database.GetRunningPrebuildsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetRunningPrebuilds(ctx)
}

func (q *querier) GetRuntimeConfig(ctx context.Context, key string) (string, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetRuntimeConfig(ctx, key)
}

func (q *querier) GetTailnetAgents(ctx context.Context, id uuid.UUID) ([]database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetAgents(ctx, id)
}

func (q *querier) GetTailnetClientsForAgent(ctx context.Context, agentID uuid.UUID) ([]database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetClientsForAgent(ctx, agentID)
}

func (q *querier) GetTailnetPeers(ctx context.Context, id uuid.UUID) ([]database.TailnetPeer, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetPeers(ctx, id)
}

func (q *querier) GetTailnetTunnelPeerBindings(ctx context.Context, srcID uuid.UUID) ([]database.GetTailnetTunnelPeerBindingsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetTunnelPeerBindings(ctx, srcID)
}

func (q *querier) GetTailnetTunnelPeerIDs(ctx context.Context, srcID uuid.UUID) ([]database.GetTailnetTunnelPeerIDsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetTunnelPeerIDs(ctx, srcID)
}

func (q *querier) GetTelemetryItem(ctx context.Context, key string) (database.TelemetryItem, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.TelemetryItem{}, err
	}
	return q.db.GetTelemetryItem(ctx, key)
}

func (q *querier) GetTelemetryItems(ctx context.Context) ([]database.TelemetryItem, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTelemetryItems(ctx)
}

func (q *querier) GetTemplateAppInsights(ctx context.Context, arg database.GetTemplateAppInsightsParams) ([]database.GetTemplateAppInsightsRow, error) {
	if err := q.authorizeTemplateInsights(ctx, arg.TemplateIDs); err != nil {
		return nil, err
	}
	return q.db.GetTemplateAppInsights(ctx, arg)
}

func (q *querier) GetTemplateAppInsightsByTemplate(ctx context.Context, arg database.GetTemplateAppInsightsByTemplateParams) ([]database.GetTemplateAppInsightsByTemplateRow, error) {
	// Only used by prometheus metrics, so we don't strictly need to check update template perms.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetTemplateAppInsightsByTemplate(ctx, arg)
}

// Only used by metrics cache.
func (q *querier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetTemplateAverageBuildTimeRow{}, err
	}
	return q.db.GetTemplateAverageBuildTime(ctx, arg)
}

func (q *querier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	return fetch(q.log, q.auth, q.db.GetTemplateByID)(ctx, id)
}

func (q *querier) GetTemplateByOrganizationAndName(ctx context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	return fetch(q.log, q.auth, q.db.GetTemplateByOrganizationAndName)(ctx, arg)
}

// Only used by metrics cache.
func (q *querier) GetTemplateDAUs(ctx context.Context, arg database.GetTemplateDAUsParams) ([]database.GetTemplateDAUsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplateDAUs(ctx, arg)
}

func (q *querier) GetTemplateInsights(ctx context.Context, arg database.GetTemplateInsightsParams) (database.GetTemplateInsightsRow, error) {
	if err := q.authorizeTemplateInsights(ctx, arg.TemplateIDs); err != nil {
		return database.GetTemplateInsightsRow{}, err
	}
	return q.db.GetTemplateInsights(ctx, arg)
}

func (q *querier) GetTemplateInsightsByInterval(ctx context.Context, arg database.GetTemplateInsightsByIntervalParams) ([]database.GetTemplateInsightsByIntervalRow, error) {
	if err := q.authorizeTemplateInsights(ctx, arg.TemplateIDs); err != nil {
		return nil, err
	}
	return q.db.GetTemplateInsightsByInterval(ctx, arg)
}

func (q *querier) GetTemplateInsightsByTemplate(ctx context.Context, arg database.GetTemplateInsightsByTemplateParams) ([]database.GetTemplateInsightsByTemplateRow, error) {
	// Only used by prometheus metrics collector. No need to check update template perms.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetTemplateInsightsByTemplate(ctx, arg)
}

func (q *querier) GetTemplateParameterInsights(ctx context.Context, arg database.GetTemplateParameterInsightsParams) ([]database.GetTemplateParameterInsightsRow, error) {
	if err := q.authorizeTemplateInsights(ctx, arg.TemplateIDs); err != nil {
		return nil, err
	}
	return q.db.GetTemplateParameterInsights(ctx, arg)
}

func (q *querier) GetTemplatePresetsWithPrebuilds(ctx context.Context, templateID uuid.NullUUID) ([]database.GetTemplatePresetsWithPrebuildsRow, error) {
	// Although this fetches presets. It filters them by prebuilds and is only of use to the prebuild system.
	// As such, we authorize this in line with other prebuild queries, not with other preset queries.

	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate); err != nil {
		return nil, err
	}
	return q.db.GetTemplatePresetsWithPrebuilds(ctx, templateID)
}

func (q *querier) GetTemplateUsageStats(ctx context.Context, arg database.GetTemplateUsageStatsParams) ([]database.TemplateUsageStat, error) {
	if err := q.authorizeTemplateInsights(ctx, arg.TemplateIDs); err != nil {
		return nil, err
	}
	return q.db.GetTemplateUsageStats(ctx, arg)
}

func (q *querier) GetTemplateVersionByID(ctx context.Context, tvid uuid.UUID) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, tvid)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionByJobID(ctx context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByJobID(ctx, jobID)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionByTemplateIDAndName(ctx context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByTemplateIDAndName(ctx, arg)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionParameters(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	// An actor can read template version parameters if they can read the related template.
	tv, err := q.db.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionParameters(ctx, templateVersionID)
}

func (q *querier) GetTemplateVersionVariables(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionVariable, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionVariables(ctx, templateVersionID)
}

func (q *querier) GetTemplateVersionWorkspaceTags(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionWorkspaceTag, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionWorkspaceTags(ctx, templateVersionID)
}

// GetTemplateVersionsByIDs is only used for workspace build data.
// The workspace is already fetched.
func (q *querier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionsByIDs(ctx, ids)
}

func (q *querier) GetTemplateVersionsByTemplateID(ctx context.Context, arg database.GetTemplateVersionsByTemplateIDParams) ([]database.TemplateVersion, error) {
	// An actor can read template versions if they can read the related template.
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return nil, err
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, template); err != nil {
		return nil, err
	}

	return q.db.GetTemplateVersionsByTemplateID(ctx, arg)
}

func (q *querier) GetTemplateVersionsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.TemplateVersion, error) {
	// An actor can read execute this query if they can read all templates.
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceTemplate.All()); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplates(ctx)
}

func (q *querier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, policy.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedTemplates(ctx, arg, prep)
}

func (q *querier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetUnexpiredLicenses(ctx)
}

func (q *querier) GetUserActivityInsights(ctx context.Context, arg database.GetUserActivityInsightsParams) ([]database.GetUserActivityInsightsRow, error) {
	// Used by insights endpoints. Need to check both for auditors and for regular users with template acl perms.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		for _, templateID := range arg.TemplateIDs {
			template, err := q.db.GetTemplateByID(ctx, templateID)
			if err != nil {
				return nil, err
			}

			if err := q.authorizeContext(ctx, policy.ActionViewInsights, template); err != nil {
				return nil, err
			}
		}
		if len(arg.TemplateIDs) == 0 {
			if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate.All()); err != nil {
				return nil, err
			}
		}
	}
	return q.db.GetUserActivityInsights(ctx, arg)
}

func (q *querier) GetUserAppearanceSettings(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := q.db.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if err := q.authorizeContext(ctx, policy.ActionReadPersonal, u); err != nil {
		return "", err
	}
	return q.db.GetUserAppearanceSettings(ctx, userID)
}

func (q *querier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByEmailOrUsername)(ctx, arg)
}

func (q *querier) GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByID)(ctx, id)
}

func (q *querier) GetUserCount(ctx context.Context, includeSystem bool) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetUserCount(ctx, includeSystem)
}

func (q *querier) GetUserLatencyInsights(ctx context.Context, arg database.GetUserLatencyInsightsParams) ([]database.GetUserLatencyInsightsRow, error) {
	// Used by insights endpoints. Need to check both for auditors and for regular users with template acl perms.
	if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate); err != nil {
		for _, templateID := range arg.TemplateIDs {
			template, err := q.db.GetTemplateByID(ctx, templateID)
			if err != nil {
				return nil, err
			}

			if err := q.authorizeContext(ctx, policy.ActionViewInsights, template); err != nil {
				return nil, err
			}
		}
		if len(arg.TemplateIDs) == 0 {
			if err := q.authorizeContext(ctx, policy.ActionViewInsights, rbac.ResourceTemplate.All()); err != nil {
				return nil, err
			}
		}
	}
	return q.db.GetUserLatencyInsights(ctx, arg)
}

func (q *querier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByLinkedID(ctx, linkedID)
}

func (q *querier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByUserIDLoginType(ctx, arg)
}

func (q *querier) GetUserLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.UserLink, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetUserLinksByUserID(ctx, userID)
}

func (q *querier) GetUserNotificationPreferences(ctx context.Context, userID uuid.UUID) ([]database.NotificationPreference, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceNotificationPreference.WithOwner(userID.String())); err != nil {
		return nil, err
	}
	return q.db.GetUserNotificationPreferences(ctx, userID)
}

func (q *querier) GetUserStatusCounts(ctx context.Context, arg database.GetUserStatusCountsParams) ([]database.GetUserStatusCountsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceUser); err != nil {
		return nil, err
	}
	return q.db.GetUserStatusCounts(ctx, arg)
}

func (q *querier) GetUserWorkspaceBuildParameters(ctx context.Context, params database.GetUserWorkspaceBuildParametersParams) ([]database.GetUserWorkspaceBuildParametersRow, error) {
	u, err := q.db.GetUserByID(ctx, params.OwnerID)
	if err != nil {
		return nil, err
	}
	// This permission is a bit strange. Reading workspace build params should be a permission
	// on the workspace. However, this use case is to autofill a user's last input
	// to some parameter. So this is kind of a "user setting". For now, this will
	// be lumped in with user personal data. Subject to change.
	if err := q.authorizeContext(ctx, policy.ActionReadPersonal, u); err != nil {
		return nil, err
	}
	return q.db.GetUserWorkspaceBuildParameters(ctx, params)
}

func (q *querier) GetUsers(ctx context.Context, arg database.GetUsersParams) ([]database.GetUsersRow, error) {
	// This does the filtering in SQL.
	prep, err := prepareSQLFilter(ctx, q.auth, policy.ActionRead, rbac.ResourceUser.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedUsers(ctx, arg, prep)
}

// GetUsersByIDs is only used for usernames on workspace return data.
// This function should be replaced by joining this data to the workspace query
// itself.
func (q *querier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	for _, uid := range ids {
		if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceUserObject(uid)); err != nil {
			return nil, err
		}
	}
	return q.db.GetUsersByIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentAndLatestBuildByAuthToken(ctx context.Context, authToken uuid.UUID) (database.GetWorkspaceAgentAndLatestBuildByAuthTokenRow, error) {
	// This is a system function
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetWorkspaceAgentAndLatestBuildByAuthTokenRow{}, err
	}
	return q.db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, authToken)
}

func (q *querier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	if _, err := q.GetWorkspaceByAgentID(ctx, id); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.GetWorkspaceAgentByID(ctx, id)
}

// GetWorkspaceAgentByInstanceID might want to be a system call? Unsure exactly,
// but this will fail. Need to figure out what AuthInstanceID is, and if it
// is essentially an auth token. But the caller using this function is not
// an authenticated user. So this authz check will fail.
func (q *querier) GetWorkspaceAgentByInstanceID(ctx context.Context, authInstanceID string) (database.WorkspaceAgent, error) {
	agent, err := q.db.GetWorkspaceAgentByInstanceID(ctx, authInstanceID)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	_, err = q.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	return agent, nil
}

func (q *querier) GetWorkspaceAgentLifecycleStateByID(ctx context.Context, id uuid.UUID) (database.GetWorkspaceAgentLifecycleStateByIDRow, error) {
	_, err := q.GetWorkspaceAgentByID(ctx, id)
	if err != nil {
		return database.GetWorkspaceAgentLifecycleStateByIDRow{}, err
	}
	return q.db.GetWorkspaceAgentLifecycleStateByID(ctx, id)
}

func (q *querier) GetWorkspaceAgentLogSourcesByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentLogSource, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentLogSourcesByAgentIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentLogsAfter(ctx context.Context, arg database.GetWorkspaceAgentLogsAfterParams) ([]database.WorkspaceAgentLog, error) {
	_, err := q.GetWorkspaceAgentByID(ctx, arg.AgentID)
	if err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentLogsAfter(ctx, arg)
}

func (q *querier) GetWorkspaceAgentMetadata(ctx context.Context, arg database.GetWorkspaceAgentMetadataParams) ([]database.WorkspaceAgentMetadatum, error) {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, arg.WorkspaceAgentID)
	if err != nil {
		return nil, err
	}

	err = q.authorizeContext(ctx, policy.ActionRead, workspace)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceAgentMetadata(ctx, arg)
}

func (q *querier) GetWorkspaceAgentPortShare(ctx context.Context, arg database.GetWorkspaceAgentPortShareParams) (database.WorkspaceAgentPortShare, error) {
	w, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.WorkspaceAgentPortShare{}, err
	}

	// reading a workspace port share is more akin to just reading the workspace.
	if err = q.authorizeContext(ctx, policy.ActionRead, w.RBACObject()); err != nil {
		return database.WorkspaceAgentPortShare{}, xerrors.Errorf("authorize context: %w", err)
	}

	return q.db.GetWorkspaceAgentPortShare(ctx, arg)
}

func (q *querier) GetWorkspaceAgentScriptTimingsByBuildID(ctx context.Context, id uuid.UUID) ([]database.GetWorkspaceAgentScriptTimingsByBuildIDRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentScriptTimingsByBuildID(ctx, id)
}

func (q *querier) GetWorkspaceAgentScriptsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentScript, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentScriptsByAgentIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentStats(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsRow, error) {
	return q.db.GetWorkspaceAgentStats(ctx, createdAfter)
}

func (q *querier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	return q.db.GetWorkspaceAgentStatsAndLabels(ctx, createdAfter)
}

func (q *querier) GetWorkspaceAgentUsageStats(ctx context.Context, createdAt time.Time) ([]database.GetWorkspaceAgentUsageStatsRow, error) {
	return q.db.GetWorkspaceAgentUsageStats(ctx, createdAt)
}

func (q *querier) GetWorkspaceAgentUsageStatsAndLabels(ctx context.Context, createdAt time.Time) ([]database.GetWorkspaceAgentUsageStatsAndLabelsRow, error) {
	return q.db.GetWorkspaceAgentUsageStatsAndLabels(ctx, createdAt)
}

// GetWorkspaceAgentsByResourceIDs
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgent, error) {
	workspace, err := q.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
}

func (q *querier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	// If we can fetch the workspace, we can fetch the apps. Use the authorized call.
	if _, err := q.GetWorkspaceByAgentID(ctx, arg.AgentID); err != nil {
		return database.WorkspaceApp{}, err
	}

	return q.db.GetWorkspaceAppByAgentIDAndSlug(ctx, arg)
}

func (q *querier) GetWorkspaceAppsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceApp, error) {
	if _, err := q.GetWorkspaceByAgentID(ctx, agentID); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsByAgentID(ctx, agentID)
}

// GetWorkspaceAppsByAgentIDs
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceBuildByID(ctx context.Context, buildID uuid.UUID) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByID(ctx, buildID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	if _, err := q.GetWorkspaceByID(ctx, build.WorkspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return build, nil
}

func (q *querier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	// Authorized fetch
	_, err = q.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	return build, nil
}

func (q *querier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, arg.WorkspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, arg)
}

func (q *querier) GetWorkspaceBuildParameters(ctx context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	// Authorized call to get the workspace build. If we can read the build,
	// we can read the params.
	_, err := q.GetWorkspaceBuildByID(ctx, workspaceBuildID)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceBuildParameters(ctx, workspaceBuildID)
}

func (q *querier) GetWorkspaceBuildStatsByTemplates(ctx context.Context, since time.Time) ([]database.GetWorkspaceBuildStatsByTemplatesRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildStatsByTemplates(ctx, since)
}

func (q *querier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, arg.WorkspaceID); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildsByWorkspaceID(ctx, arg)
}

// Telemetry related functions. These functions are system functions for returning
// telemetry data. Never called by a user.

func (q *querier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByAgentID)(ctx, agentID)
}

func (q *querier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByID)(ctx, id)
}

func (q *querier) GetWorkspaceByOwnerIDAndName(ctx context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByOwnerIDAndName)(ctx, arg)
}

func (q *querier) GetWorkspaceByWorkspaceAppID(ctx context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByWorkspaceAppID)(ctx, workspaceAppID)
}

func (q *querier) GetWorkspaceModulesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceModule, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceModulesByJobID(ctx, jobID)
}

func (q *querier) GetWorkspaceModulesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceModule, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceModulesCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceProxies(ctx context.Context) ([]database.WorkspaceProxy, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, func(ctx context.Context, _ interface{}) ([]database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxies(ctx)
	})(ctx, nil)
}

func (q *querier) GetWorkspaceProxyByHostname(ctx context.Context, params database.GetWorkspaceProxyByHostnameParams) (database.WorkspaceProxy, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return database.WorkspaceProxy{}, err
	}
	return q.db.GetWorkspaceProxyByHostname(ctx, params)
}

func (q *querier) GetWorkspaceProxyByID(ctx context.Context, id uuid.UUID) (database.WorkspaceProxy, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceProxyByID)(ctx, id)
}

func (q *querier) GetWorkspaceProxyByName(ctx context.Context, name string) (database.WorkspaceProxy, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceProxyByName)(ctx, name)
}

func (q *querier) GetWorkspaceResourceByID(ctx context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	// TODO: Optimize this
	resource, err := q.db.GetWorkspaceResourceByID(ctx, id)
	if err != nil {
		return database.WorkspaceResource{}, err
	}

	_, err = q.GetProvisionerJobByID(ctx, resource.JobID)
	if err != nil {
		return database.WorkspaceResource{}, err
	}

	return resource, nil
}

// GetWorkspaceResourceMetadataByResourceIDs is only used for build data.
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourceMetadataCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	job, err := q.db.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var obj rbac.Objecter
	switch job.Type {
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// We don't need to do an authorized check, but this helper function
		// handles the job type for us.
		// TODO: Do not duplicate auth checks.
		tv, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return nil, err
		}
		if !tv.TemplateID.Valid {
			// Orphaned template version
			obj = tv.RBACObjectNoTemplate()
		} else {
			template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
			if err != nil {
				return nil, err
			}
			obj = template.RBACObject()
		}
	case database.ProvisionerJobTypeWorkspaceBuild:
		build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
		if err != nil {
			return nil, err
		}
		workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return nil, err
		}
		obj = workspace
	default:
		return nil, xerrors.Errorf("unknown job type: %s", job.Type)
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, obj); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobID(ctx, jobID)
}

// GetWorkspaceResourcesByJobIDs is only used for workspace build data.
// The workspace is already fetched.
// TODO: Find a way to replace this with proper authz.
func (q *querier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *querier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaceUniqueOwnerCountByTemplateIDs(ctx context.Context, templateIds []uuid.UUID) ([]database.GetWorkspaceUniqueOwnerCountByTemplateIDsRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceUniqueOwnerCountByTemplateIDs(ctx, templateIds)
}

func (q *querier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, policy.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedWorkspaces(ctx, arg, prep)
}

func (q *querier) GetWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]database.GetWorkspacesAndAgentsByOwnerIDRow, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, policy.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedWorkspacesAndAgentsByOwnerID(ctx, ownerID, prep)
}

func (q *querier) GetWorkspacesByTemplateID(ctx context.Context, templateID uuid.UUID) ([]database.WorkspaceTable, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspacesByTemplateID(ctx, templateID)
}

func (q *querier) GetWorkspacesEligibleForTransition(ctx context.Context, now time.Time) ([]database.GetWorkspacesEligibleForTransitionRow, error) {
	return q.db.GetWorkspacesEligibleForTransition(ctx, now)
}

func (q *querier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return insert(q.log, q.auth,
		rbac.ResourceApiKey.WithOwner(arg.UserID.String()),
		q.db.InsertAPIKey)(ctx, arg)
}

func (q *querier) InsertAllUsersGroup(ctx context.Context, organizationID uuid.UUID) (database.Group, error) {
	// This method creates a new group.
	return insert(q.log, q.auth, rbac.ResourceGroup.InOrg(organizationID), q.db.InsertAllUsersGroup)(ctx, organizationID)
}

func (q *querier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	return insert(q.log, q.auth, rbac.ResourceAuditLog, q.db.InsertAuditLog)(ctx, arg)
}

func (q *querier) InsertCryptoKey(ctx context.Context, arg database.InsertCryptoKeyParams) (database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceCryptoKey); err != nil {
		return database.CryptoKey{}, err
	}
	return q.db.InsertCryptoKey(ctx, arg)
}

func (q *querier) InsertCustomRole(ctx context.Context, arg database.InsertCustomRoleParams) (database.CustomRole, error) {
	// Org and site role upsert share the same query. So switch the assertion based on the org uuid.
	if !arg.OrganizationID.Valid || arg.OrganizationID.UUID == uuid.Nil {
		return database.CustomRole{}, NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")}
	}
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceAssignOrgRole.InOrg(arg.OrganizationID.UUID)); err != nil {
		return database.CustomRole{}, err
	}

	if err := q.customRoleCheck(ctx, database.CustomRole{
		Name:            arg.Name,
		DisplayName:     arg.DisplayName,
		SitePermissions: arg.SitePermissions,
		OrgPermissions:  arg.OrgPermissions,
		UserPermissions: arg.UserPermissions,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		OrganizationID:  arg.OrganizationID,
		ID:              uuid.New(),
	}); err != nil {
		return database.CustomRole{}, err
	}
	return q.db.InsertCustomRole(ctx, arg)
}

func (q *querier) InsertDBCryptKey(ctx context.Context, arg database.InsertDBCryptKeyParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDBCryptKey(ctx, arg)
}

func (q *querier) InsertDERPMeshKey(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDERPMeshKey(ctx, value)
}

func (q *querier) InsertDeploymentID(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDeploymentID(ctx, value)
}

func (q *querier) InsertExternalAuthLink(ctx context.Context, arg database.InsertExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	return insertWithAction(q.log, q.auth, rbac.ResourceUser.WithID(arg.UserID).WithOwner(arg.UserID.String()), policy.ActionUpdatePersonal, q.db.InsertExternalAuthLink)(ctx, arg)
}

func (q *querier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	return insert(q.log, q.auth, rbac.ResourceFile.WithOwner(arg.CreatedBy.String()), q.db.InsertFile)(ctx, arg)
}

func (q *querier) InsertGitSSHKey(ctx context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	return insertWithAction(q.log, q.auth, rbac.ResourceUser.WithOwner(arg.UserID.String()).WithID(arg.UserID), policy.ActionUpdatePersonal, q.db.InsertGitSSHKey)(ctx, arg)
}

func (q *querier) InsertGroup(ctx context.Context, arg database.InsertGroupParams) (database.Group, error) {
	return insert(q.log, q.auth, rbac.ResourceGroup.InOrg(arg.OrganizationID), q.db.InsertGroup)(ctx, arg)
}

func (q *querier) InsertGroupMember(ctx context.Context, arg database.InsertGroupMemberParams) error {
	fetch := func(ctx context.Context, arg database.InsertGroupMemberParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.GroupID)
	}
	return update(q.log, q.auth, fetch, q.db.InsertGroupMember)(ctx, arg)
}

func (q *querier) InsertInboxNotification(ctx context.Context, arg database.InsertInboxNotificationParams) (database.InboxNotification, error) {
	return insert(q.log, q.auth, rbac.ResourceInboxNotification.WithOwner(arg.UserID.String()), q.db.InsertInboxNotification)(ctx, arg)
}

func (q *querier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceLicense); err != nil {
		return database.License{}, err
	}
	return q.db.InsertLicense(ctx, arg)
}

func (q *querier) InsertMemoryResourceMonitor(ctx context.Context, arg database.InsertMemoryResourceMonitorParams) (database.WorkspaceAgentMemoryResourceMonitor, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return database.WorkspaceAgentMemoryResourceMonitor{}, err
	}

	return q.db.InsertMemoryResourceMonitor(ctx, arg)
}

func (q *querier) InsertMissingGroups(ctx context.Context, arg database.InsertMissingGroupsParams) ([]database.Group, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.InsertMissingGroups(ctx, arg)
}

func (q *querier) InsertOAuth2ProviderApp(ctx context.Context, arg database.InsertOAuth2ProviderAppParams) (database.OAuth2ProviderApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceOauth2App); err != nil {
		return database.OAuth2ProviderApp{}, err
	}
	return q.db.InsertOAuth2ProviderApp(ctx, arg)
}

func (q *querier) InsertOAuth2ProviderAppCode(ctx context.Context, arg database.InsertOAuth2ProviderAppCodeParams) (database.OAuth2ProviderAppCode, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate,
		rbac.ResourceOauth2AppCodeToken.WithOwner(arg.UserID.String())); err != nil {
		return database.OAuth2ProviderAppCode{}, err
	}
	return q.db.InsertOAuth2ProviderAppCode(ctx, arg)
}

func (q *querier) InsertOAuth2ProviderAppSecret(ctx context.Context, arg database.InsertOAuth2ProviderAppSecretParams) (database.OAuth2ProviderAppSecret, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceOauth2AppSecret); err != nil {
		return database.OAuth2ProviderAppSecret{}, err
	}
	return q.db.InsertOAuth2ProviderAppSecret(ctx, arg)
}

func (q *querier) InsertOAuth2ProviderAppToken(ctx context.Context, arg database.InsertOAuth2ProviderAppTokenParams) (database.OAuth2ProviderAppToken, error) {
	key, err := q.db.GetAPIKeyByID(ctx, arg.APIKeyID)
	if err != nil {
		return database.OAuth2ProviderAppToken{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceOauth2AppCodeToken.WithOwner(key.UserID.String())); err != nil {
		return database.OAuth2ProviderAppToken{}, err
	}
	return q.db.InsertOAuth2ProviderAppToken(ctx, arg)
}

func (q *querier) InsertOrganization(ctx context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	return insert(q.log, q.auth, rbac.ResourceOrganization, q.db.InsertOrganization)(ctx, arg)
}

func (q *querier) InsertOrganizationMember(ctx context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	orgRoles, err := q.convertToOrganizationRoles(arg.OrganizationID, arg.Roles)
	if err != nil {
		return database.OrganizationMember{}, xerrors.Errorf("converting to organization roles: %w", err)
	}

	// All roles are added roles. Org member is always implied.
	addedRoles := append(orgRoles, rbac.ScopedRoleOrgMember(arg.OrganizationID))
	err = q.canAssignRoles(ctx, arg.OrganizationID, addedRoles, []rbac.RoleIdentifier{})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	obj := rbac.ResourceOrganizationMember.InOrg(arg.OrganizationID).WithID(arg.UserID)
	return insert(q.log, q.auth, obj, q.db.InsertOrganizationMember)(ctx, arg)
}

func (q *querier) InsertPreset(ctx context.Context, arg database.InsertPresetParams) (database.TemplateVersionPreset, error) {
	err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTemplate)
	if err != nil {
		return database.TemplateVersionPreset{}, err
	}

	return q.db.InsertPreset(ctx, arg)
}

func (q *querier) InsertPresetParameters(ctx context.Context, arg database.InsertPresetParametersParams) ([]database.TemplateVersionPresetParameter, error) {
	err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTemplate)
	if err != nil {
		return nil, err
	}

	return q.db.InsertPresetParameters(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.InsertProvisionerJob(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	// if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.InsertProvisionerJobLogs(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJobTimings(ctx context.Context, arg database.InsertProvisionerJobTimingsParams) ([]database.ProvisionerJobTiming, error) {
	// if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.InsertProvisionerJobTimings(ctx, arg)
}

func (q *querier) InsertProvisionerKey(ctx context.Context, arg database.InsertProvisionerKeyParams) (database.ProvisionerKey, error) {
	return insert(q.log, q.auth, rbac.ResourceProvisionerDaemon.InOrg(arg.OrganizationID).WithID(arg.ID), q.db.InsertProvisionerKey)(ctx, arg)
}

func (q *querier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.InsertReplica(ctx, arg)
}

func (q *querier) InsertTelemetryItemIfNotExists(ctx context.Context, arg database.InsertTelemetryItemIfNotExistsParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertTelemetryItemIfNotExists(ctx, arg)
}

func (q *querier) InsertTemplate(ctx context.Context, arg database.InsertTemplateParams) error {
	obj := rbac.ResourceTemplate.InOrg(arg.OrganizationID)
	if err := q.authorizeContext(ctx, policy.ActionCreate, obj); err != nil {
		return err
	}
	return q.db.InsertTemplate(ctx, arg)
}

func (q *querier) InsertTemplateVersion(ctx context.Context, arg database.InsertTemplateVersionParams) error {
	if !arg.TemplateID.Valid {
		// Making a new template version is the same permission as creating a new template.
		err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceTemplate.InOrg(arg.OrganizationID))
		if err != nil {
			return err
		}
	} else {
		// Must do an authorized fetch to prevent leaking template ids this way.
		tpl, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID)
		if err != nil {
			return err
		}
		// Check the create permission on the template.
		err = q.authorizeContext(ctx, policy.ActionCreate, tpl)
		if err != nil {
			return err
		}
	}

	return q.db.InsertTemplateVersion(ctx, arg)
}

func (q *querier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionParameter{}, err
	}
	return q.db.InsertTemplateVersionParameter(ctx, arg)
}

func (q *querier) InsertTemplateVersionVariable(ctx context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionVariable{}, err
	}
	return q.db.InsertTemplateVersionVariable(ctx, arg)
}

func (q *querier) InsertTemplateVersionWorkspaceTag(ctx context.Context, arg database.InsertTemplateVersionWorkspaceTagParams) (database.TemplateVersionWorkspaceTag, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionWorkspaceTag{}, err
	}
	return q.db.InsertTemplateVersionWorkspaceTag(ctx, arg)
}

func (q *querier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	// Always check if the assigned roles can actually be assigned by this actor.
	impliedRoles := append([]rbac.RoleIdentifier{rbac.RoleMember()}, q.convertToDeploymentRoles(arg.RBACRoles)...)
	err := q.canAssignRoles(ctx, uuid.Nil, impliedRoles, []rbac.RoleIdentifier{})
	if err != nil {
		return database.User{}, err
	}
	obj := rbac.ResourceUser
	return insert(q.log, q.auth, obj, q.db.InsertUser)(ctx, arg)
}

func (q *querier) InsertUserGroupsByID(ctx context.Context, arg database.InsertUserGroupsByIDParams) ([]uuid.UUID, error) {
	// This is used by OIDC sync. So only used by a system user.
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.InsertUserGroupsByID(ctx, arg)
}

func (q *querier) InsertUserGroupsByName(ctx context.Context, arg database.InsertUserGroupsByNameParams) error {
	// This will add the user to all named groups. This counts as updating a group.
	// NOTE: instead of checking if the user has permission to update each group, we instead
	// check if the user has permission to update *a* group in the org.
	fetch := func(ctx context.Context, arg database.InsertUserGroupsByNameParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return update(q.log, q.auth, fetch, q.db.InsertUserGroupsByName)(ctx, arg)
}

// TODO: Should this be in system.go?
func (q *querier) InsertUserLink(ctx context.Context, arg database.InsertUserLinkParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceUserObject(arg.UserID)); err != nil {
		return database.UserLink{}, err
	}
	return q.db.InsertUserLink(ctx, arg)
}

func (q *querier) InsertVolumeResourceMonitor(ctx context.Context, arg database.InsertVolumeResourceMonitorParams) (database.WorkspaceAgentVolumeResourceMonitor, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return database.WorkspaceAgentVolumeResourceMonitor{}, err
	}

	return q.db.InsertVolumeResourceMonitor(ctx, arg)
}

func (q *querier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.WorkspaceTable, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	tpl, err := q.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return database.WorkspaceTable{}, xerrors.Errorf("verify template by id: %w", err)
	}
	if err := q.authorizeContext(ctx, policy.ActionUse, tpl); err != nil {
		return database.WorkspaceTable{}, xerrors.Errorf("use template for workspace: %w", err)
	}

	return insert(q.log, q.auth, obj, q.db.InsertWorkspace)(ctx, arg)
}

func (q *querier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.InsertWorkspaceAgent(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentLogSources(ctx context.Context, arg database.InsertWorkspaceAgentLogSourcesParams) ([]database.WorkspaceAgentLogSource, error) {
	// TODO: This is used by the agent, should we have an rbac check here?
	return q.db.InsertWorkspaceAgentLogSources(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentLogs(ctx context.Context, arg database.InsertWorkspaceAgentLogsParams) ([]database.WorkspaceAgentLog, error) {
	// TODO: This is used by the agent, should we have an rbac check here?
	return q.db.InsertWorkspaceAgentLogs(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentMetadata(ctx context.Context, arg database.InsertWorkspaceAgentMetadataParams) error {
	// We don't check for workspace ownership here since the agent metadata may
	// be associated with an orphaned agent used by a dry run build.
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}

	return q.db.InsertWorkspaceAgentMetadata(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentScriptTimings(ctx context.Context, arg database.InsertWorkspaceAgentScriptTimingsParams) (database.WorkspaceAgentScriptTiming, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgentScriptTiming{}, err
	}
	return q.db.InsertWorkspaceAgentScriptTimings(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentScripts(ctx context.Context, arg database.InsertWorkspaceAgentScriptsParams) ([]database.WorkspaceAgentScript, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return []database.WorkspaceAgentScript{}, err
	}
	return q.db.InsertWorkspaceAgentScripts(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentStats(ctx context.Context, arg database.InsertWorkspaceAgentStatsParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}

	return q.db.InsertWorkspaceAgentStats(ctx, arg)
}

func (q *querier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceApp{}, err
	}
	return q.db.InsertWorkspaceApp(ctx, arg)
}

func (q *querier) InsertWorkspaceAppStats(ctx context.Context, arg database.InsertWorkspaceAppStatsParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertWorkspaceAppStats(ctx, arg)
}

func (q *querier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) error {
	w, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace by id: %w", err)
	}

	var action policy.Action = policy.ActionWorkspaceStart
	if arg.Transition == database.WorkspaceTransitionDelete {
		action = policy.ActionDelete
	} else if arg.Transition == database.WorkspaceTransitionStop {
		action = policy.ActionWorkspaceStop
	}

	if err = q.authorizeContext(ctx, action, w); err != nil {
		return xerrors.Errorf("authorize context: %w", err)
	}

	// If we're starting a workspace we need to check the template.
	if arg.Transition == database.WorkspaceTransitionStart {
		t, err := q.db.GetTemplateByID(ctx, w.TemplateID)
		if err != nil {
			return xerrors.Errorf("get template by id: %w", err)
		}

		accessControl := (*q.acs.Load()).GetTemplateAccessControl(t)

		// If the template requires the active version we need to check if
		// the user is a template admin. If they aren't and are attempting
		// to use a non-active version then we must fail the request.
		if accessControl.RequireActiveVersion {
			if arg.TemplateVersionID != t.ActiveVersionID {
				if err = q.authorizeContext(ctx, policy.ActionUpdate, t); err != nil {
					return xerrors.Errorf("cannot use non-active version: %w", err)
				}
			}
		}
	}

	return q.db.InsertWorkspaceBuild(ctx, arg)
}

func (q *querier) InsertWorkspaceBuildParameters(ctx context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	// TODO: Optimize this. We always have the workspace and build already fetched.
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.WorkspaceBuildID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace)
	if err != nil {
		return err
	}

	return q.db.InsertWorkspaceBuildParameters(ctx, arg)
}

func (q *querier) InsertWorkspaceModule(ctx context.Context, arg database.InsertWorkspaceModuleParams) (database.WorkspaceModule, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceModule{}, err
	}
	return q.db.InsertWorkspaceModule(ctx, arg)
}

func (q *querier) InsertWorkspaceProxy(ctx context.Context, arg database.InsertWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	return insert(q.log, q.auth, rbac.ResourceWorkspaceProxy, q.db.InsertWorkspaceProxy)(ctx, arg)
}

func (q *querier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceResource{}, err
	}
	return q.db.InsertWorkspaceResource(ctx, arg)
}

func (q *querier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.InsertWorkspaceResourceMetadata(ctx, arg)
}

func (q *querier) ListProvisionerKeysByOrganization(ctx context.Context, organizationID uuid.UUID) ([]database.ProvisionerKey, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.ListProvisionerKeysByOrganization)(ctx, organizationID)
}

func (q *querier) ListProvisionerKeysByOrganizationExcludeReserved(ctx context.Context, organizationID uuid.UUID) ([]database.ProvisionerKey, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.ListProvisionerKeysByOrganizationExcludeReserved)(ctx, organizationID)
}

func (q *querier) ListWorkspaceAgentPortShares(ctx context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgentPortShare, error) {
	workspace, err := q.db.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// listing port shares is more akin to reading the workspace.
	if err := q.authorizeContext(ctx, policy.ActionRead, workspace); err != nil {
		return nil, err
	}

	return q.db.ListWorkspaceAgentPortShares(ctx, workspaceID)
}

func (q *querier) OIDCClaimFieldValues(ctx context.Context, args database.OIDCClaimFieldValuesParams) ([]string, error) {
	resource := rbac.ResourceIdpsyncSettings
	if args.OrganizationID != uuid.Nil {
		resource = resource.InOrg(args.OrganizationID)
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, resource); err != nil {
		return nil, err
	}
	return q.db.OIDCClaimFieldValues(ctx, args)
}

func (q *querier) OIDCClaimFields(ctx context.Context, organizationID uuid.UUID) ([]string, error) {
	resource := rbac.ResourceIdpsyncSettings
	if organizationID != uuid.Nil {
		resource = resource.InOrg(organizationID)
	}

	if err := q.authorizeContext(ctx, policy.ActionRead, resource); err != nil {
		return nil, err
	}
	return q.db.OIDCClaimFields(ctx, organizationID)
}

func (q *querier) OrganizationMembers(ctx context.Context, arg database.OrganizationMembersParams) ([]database.OrganizationMembersRow, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.OrganizationMembers)(ctx, arg)
}

func (q *querier) PaginatedOrganizationMembers(ctx context.Context, arg database.PaginatedOrganizationMembersParams) ([]database.PaginatedOrganizationMembersRow, error) {
	// Required to have permission to read all members in the organization
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceOrganizationMember.InOrg(arg.OrganizationID)); err != nil {
		return nil, err
	}
	return q.db.PaginatedOrganizationMembers(ctx, arg)
}

func (q *querier) ReduceWorkspaceAgentShareLevelToAuthenticatedByTemplate(ctx context.Context, templateID uuid.UUID) error {
	template, err := q.db.GetTemplateByID(ctx, templateID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return err
	}

	return q.db.ReduceWorkspaceAgentShareLevelToAuthenticatedByTemplate(ctx, templateID)
}

func (q *querier) RegisterWorkspaceProxy(ctx context.Context, arg database.RegisterWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	fetch := func(ctx context.Context, arg database.RegisterWorkspaceProxyParams) (database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxyByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.RegisterWorkspaceProxy)(ctx, arg)
}

func (q *querier) RemoveUserFromAllGroups(ctx context.Context, userID uuid.UUID) error {
	// This is a system function to clear user groups in group sync.
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.RemoveUserFromAllGroups(ctx, userID)
}

func (q *querier) RemoveUserFromGroups(ctx context.Context, arg database.RemoveUserFromGroupsParams) ([]uuid.UUID, error) {
	// This is a system function to clear user groups in group sync.
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.RemoveUserFromGroups(ctx, arg)
}

func (q *querier) RevokeDBCryptKey(ctx context.Context, activeKeyDigest string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.RevokeDBCryptKey(ctx, activeKeyDigest)
}

func (q *querier) TryAcquireLock(ctx context.Context, id int64) (bool, error) {
	return q.db.TryAcquireLock(ctx, id)
}

func (q *querier) UnarchiveTemplateVersion(ctx context.Context, arg database.UnarchiveTemplateVersionParams) error {
	v, err := q.db.GetTemplateVersionByID(ctx, arg.TemplateVersionID)
	if err != nil {
		return err
	}

	tpl, err := q.db.GetTemplateByID(ctx, v.TemplateID.UUID)
	if err != nil {
		return err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, tpl); err != nil {
		return err
	}
	return q.db.UnarchiveTemplateVersion(ctx, arg)
}

func (q *querier) UnfavoriteWorkspace(ctx context.Context, id uuid.UUID) error {
	fetch := func(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, id)
	}
	return update(q.log, q.auth, fetch, q.db.UnfavoriteWorkspace)(ctx, id)
}

func (q *querier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.db.GetAPIKeyByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateAPIKeyByID)(ctx, arg)
}

func (q *querier) UpdateCryptoKeyDeletesAt(ctx context.Context, arg database.UpdateCryptoKeyDeletesAtParams) (database.CryptoKey, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceCryptoKey); err != nil {
		return database.CryptoKey{}, err
	}
	return q.db.UpdateCryptoKeyDeletesAt(ctx, arg)
}

func (q *querier) UpdateCustomRole(ctx context.Context, arg database.UpdateCustomRoleParams) (database.CustomRole, error) {
	if !arg.OrganizationID.Valid || arg.OrganizationID.UUID == uuid.Nil {
		return database.CustomRole{}, NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")}
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceAssignOrgRole.InOrg(arg.OrganizationID.UUID)); err != nil {
		return database.CustomRole{}, err
	}

	if err := q.customRoleCheck(ctx, database.CustomRole{
		Name:            arg.Name,
		DisplayName:     arg.DisplayName,
		SitePermissions: arg.SitePermissions,
		OrgPermissions:  arg.OrgPermissions,
		UserPermissions: arg.UserPermissions,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		OrganizationID:  arg.OrganizationID,
		ID:              uuid.New(),
	}); err != nil {
		return database.CustomRole{}, err
	}
	return q.db.UpdateCustomRole(ctx, arg)
}

func (q *querier) UpdateExternalAuthLink(ctx context.Context, arg database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	fetch := func(ctx context.Context, arg database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
		return q.db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{UserID: arg.UserID, ProviderID: arg.ProviderID})
	}
	return fetchAndQuery(q.log, q.auth, policy.ActionUpdatePersonal, fetch, q.db.UpdateExternalAuthLink)(ctx, arg)
}

func (q *querier) UpdateExternalAuthLinkRefreshToken(ctx context.Context, arg database.UpdateExternalAuthLinkRefreshTokenParams) error {
	fetch := func(ctx context.Context, arg database.UpdateExternalAuthLinkRefreshTokenParams) (database.ExternalAuthLink, error) {
		return q.db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{UserID: arg.UserID, ProviderID: arg.ProviderID})
	}
	return fetchAndExec(q.log, q.auth, policy.ActionUpdatePersonal, fetch, q.db.UpdateExternalAuthLinkRefreshToken)(ctx, arg)
}

func (q *querier) UpdateGitSSHKey(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	fetch := func(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
		return q.db.GetGitSSHKey(ctx, arg.UserID)
	}
	return fetchAndQuery(q.log, q.auth, policy.ActionUpdatePersonal, fetch, q.db.UpdateGitSSHKey)(ctx, arg)
}

func (q *querier) UpdateGroupByID(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	fetch := func(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGroupByID)(ctx, arg)
}

func (q *querier) UpdateInactiveUsersToDormant(ctx context.Context, lastSeenAfter database.UpdateInactiveUsersToDormantParams) ([]database.UpdateInactiveUsersToDormantRow, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.UpdateInactiveUsersToDormant(ctx, lastSeenAfter)
}

func (q *querier) UpdateInboxNotificationReadStatus(ctx context.Context, args database.UpdateInboxNotificationReadStatusParams) error {
	fetchFunc := func(ctx context.Context, args database.UpdateInboxNotificationReadStatusParams) (database.InboxNotification, error) {
		return q.db.GetInboxNotificationByID(ctx, args.ID)
	}

	return update(q.log, q.auth, fetchFunc, q.db.UpdateInboxNotificationReadStatus)(ctx, args)
}

func (q *querier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// Authorized fetch will check that the actor has read access to the org member since the org member is returned.
	member, err := database.ExpectOne(q.OrganizationMembers(ctx, database.OrganizationMembersParams{
		OrganizationID: arg.OrgID,
		UserID:         arg.UserID,
		IncludeSystem:  false,
	}))
	if err != nil {
		return database.OrganizationMember{}, err
	}

	originalRoles, err := q.convertToOrganizationRoles(member.OrganizationMember.OrganizationID, member.OrganizationMember.Roles)
	if err != nil {
		return database.OrganizationMember{}, xerrors.Errorf("convert original roles: %w", err)
	}

	// The 'rbac' package expects role names to be scoped.
	// Convert the argument roles for validation.
	scopedGranted, err := q.convertToOrganizationRoles(arg.OrgID, arg.GrantedRoles)
	if err != nil {
		return database.OrganizationMember{}, err
	}

	// The org member role is always implied.
	impliedTypes := append(scopedGranted, rbac.ScopedRoleOrgMember(arg.OrgID))

	added, removed := rbac.ChangeRoleSet(originalRoles, impliedTypes)
	err = q.canAssignRoles(ctx, arg.OrgID, added, removed)
	if err != nil {
		return database.OrganizationMember{}, err
	}

	return q.db.UpdateMemberRoles(ctx, arg)
}

func (q *querier) UpdateMemoryResourceMonitor(ctx context.Context, arg database.UpdateMemoryResourceMonitorParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return err
	}

	return q.db.UpdateMemoryResourceMonitor(ctx, arg)
}

func (q *querier) UpdateNotificationTemplateMethodByID(ctx context.Context, arg database.UpdateNotificationTemplateMethodByIDParams) (database.NotificationTemplate, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceNotificationTemplate); err != nil {
		return database.NotificationTemplate{}, err
	}
	return q.db.UpdateNotificationTemplateMethodByID(ctx, arg)
}

func (q *querier) UpdateOAuth2ProviderAppByID(ctx context.Context, arg database.UpdateOAuth2ProviderAppByIDParams) (database.OAuth2ProviderApp, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceOauth2App); err != nil {
		return database.OAuth2ProviderApp{}, err
	}
	return q.db.UpdateOAuth2ProviderAppByID(ctx, arg)
}

func (q *querier) UpdateOAuth2ProviderAppSecretByID(ctx context.Context, arg database.UpdateOAuth2ProviderAppSecretByIDParams) (database.OAuth2ProviderAppSecret, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceOauth2AppSecret); err != nil {
		return database.OAuth2ProviderAppSecret{}, err
	}
	return q.db.UpdateOAuth2ProviderAppSecretByID(ctx, arg)
}

func (q *querier) UpdateOrganization(ctx context.Context, arg database.UpdateOrganizationParams) (database.Organization, error) {
	fetch := func(ctx context.Context, arg database.UpdateOrganizationParams) (database.Organization, error) {
		return q.db.GetOrganizationByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateOrganization)(ctx, arg)
}

func (q *querier) UpdateOrganizationDeletedByID(ctx context.Context, arg database.UpdateOrganizationDeletedByIDParams) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			ID:        id,
			UpdatedAt: dbtime.Now(),
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetOrganizationByID, deleteF)(ctx, arg.ID)
}

func (q *querier) UpdateProvisionerDaemonLastSeenAt(ctx context.Context, arg database.UpdateProvisionerDaemonLastSeenAtParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceProvisionerDaemon); err != nil {
		return err
	}
	return q.db.UpdateProvisionerDaemonLastSeenAt(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	// if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return err
	// }
	return q.db.UpdateProvisionerJobByID(ctx, arg)
}

func (q *querier) UpdateProvisionerJobWithCancelByID(ctx context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
	job, err := q.db.GetProvisionerJobByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		build, err := q.db.GetWorkspaceBuildByJobID(ctx, arg.ID)
		if err != nil {
			return err
		}
		workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return err
		}

		template, err := q.db.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			return err
		}

		// Template can specify if cancels are allowed.
		// Would be nice to have a way in the rbac rego to do this.
		if !template.AllowUserCancelWorkspaceJobs {
			// Only owners can cancel workspace builds
			actor, ok := ActorFromContext(ctx)
			if !ok {
				return NoActorError
			}
			if !slice.Contains(actor.Roles.Names(), rbac.RoleOwner()) {
				return xerrors.Errorf("only owners can cancel workspace builds")
			}
		}

		err = q.authorizeContext(ctx, policy.ActionUpdate, workspace)
		if err != nil {
			return err
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		templateVersion, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return err
		}

		if templateVersion.TemplateID.Valid {
			template, err := q.db.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
			if err != nil {
				return err
			}
			err = q.authorizeContext(ctx, policy.ActionUpdate, templateVersion.RBACObject(template))
			if err != nil {
				return err
			}
		} else {
			err = q.authorizeContext(ctx, policy.ActionUpdate, templateVersion.RBACObjectNoTemplate())
			if err != nil {
				return err
			}
		}
	default:
		return xerrors.Errorf("unknown job type: %q", job.Type)
	}
	return q.db.UpdateProvisionerJobWithCancelByID(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) UpdateProvisionerJobWithCompleteByID(ctx context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	// if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return err
	// }
	return q.db.UpdateProvisionerJobWithCompleteByID(ctx, arg)
}

func (q *querier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.UpdateReplica(ctx, arg)
}

func (q *querier) UpdateTailnetPeerStatusByCoordinator(ctx context.Context, arg database.UpdateTailnetPeerStatusByCoordinatorParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.UpdateTailnetPeerStatusByCoordinator(ctx, arg)
}

func (q *querier) UpdateTemplateACLByID(ctx context.Context, arg database.UpdateTemplateACLByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	// UpdateTemplateACL uses the ActionCreate action. Only users that can create the template
	// may update the ACL.
	return fetchAndExec(q.log, q.auth, policy.ActionCreate, fetch, q.db.UpdateTemplateACLByID)(ctx, arg)
}

func (q *querier) UpdateTemplateAccessControlByID(ctx context.Context, arg database.UpdateTemplateAccessControlByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateAccessControlByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateTemplateAccessControlByID)(ctx, arg)
}

func (q *querier) UpdateTemplateActiveVersionByID(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateTemplateActiveVersionByID)(ctx, arg)
}

// Deprecated: use SoftDeleteTemplateByID instead.
func (q *querier) UpdateTemplateDeletedByID(ctx context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	return q.SoftDeleteTemplateByID(ctx, arg.ID)
}

func (q *querier) UpdateTemplateMetaByID(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateTemplateMetaByID)(ctx, arg)
}

func (q *querier) UpdateTemplateScheduleByID(ctx context.Context, arg database.UpdateTemplateScheduleByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateScheduleByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateTemplateScheduleByID)(ctx, arg)
}

func (q *querier) UpdateTemplateVersionByID(ctx context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	// An actor is allowed to update the template version if they are authorized to update the template.
	tv, err := q.db.GetTemplateVersionByID(ctx, arg.ID)
	if err != nil {
		return err
	}
	var obj rbac.Objecter
	if !tv.TemplateID.Valid {
		obj = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		tpl, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
		if err != nil {
			return err
		}
		obj = tpl
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionByID(ctx, arg)
}

func (q *querier) UpdateTemplateVersionDescriptionByJobID(ctx context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
	// An actor is allowed to update the template version description if they are authorized to update the template.
	tv, err := q.db.GetTemplateVersionByJobID(ctx, arg.JobID)
	if err != nil {
		return err
	}
	var obj rbac.Objecter
	if !tv.TemplateID.Valid {
		obj = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		tpl, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
		if err != nil {
			return err
		}
		obj = tpl
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionDescriptionByJobID(ctx, arg)
}

func (q *querier) UpdateTemplateVersionExternalAuthProvidersByJobID(ctx context.Context, arg database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams) error {
	// An actor is allowed to update the template version external auth providers if they are authorized to update the template.
	tv, err := q.db.GetTemplateVersionByJobID(ctx, arg.JobID)
	if err != nil {
		return err
	}
	var obj rbac.Objecter
	if !tv.TemplateID.Valid {
		obj = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		tpl, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
		if err != nil {
			return err
		}
		obj = tpl
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionExternalAuthProvidersByJobID(ctx, arg)
}

func (q *querier) UpdateTemplateWorkspacesLastUsedAt(ctx context.Context, arg database.UpdateTemplateWorkspacesLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateWorkspacesLastUsedAtParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.TemplateID)
	}

	return fetchAndExec(q.log, q.auth, policy.ActionUpdate, fetch, q.db.UpdateTemplateWorkspacesLastUsedAt)(ctx, arg)
}

func (q *querier) UpdateUserAppearanceSettings(ctx context.Context, arg database.UpdateUserAppearanceSettingsParams) (database.UserConfig, error) {
	u, err := q.db.GetUserByID(ctx, arg.UserID)
	if err != nil {
		return database.UserConfig{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdatePersonal, u); err != nil {
		return database.UserConfig{}, err
	}
	return q.db.UpdateUserAppearanceSettings(ctx, arg)
}

func (q *querier) UpdateUserDeletedByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetUserByID, q.db.UpdateUserDeletedByID)(ctx, id)
}

func (q *querier) UpdateUserGithubComUserID(ctx context.Context, arg database.UpdateUserGithubComUserIDParams) error {
	user, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdatePersonal, user)
	if err != nil {
		// System user can also update
		err = q.authorizeContext(ctx, policy.ActionUpdate, user)
		if err != nil {
			return err
		}
	}
	return q.db.UpdateUserGithubComUserID(ctx, arg)
}

func (q *querier) UpdateUserHashedOneTimePasscode(ctx context.Context, arg database.UpdateUserHashedOneTimePasscodeParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}

	return q.db.UpdateUserHashedOneTimePasscode(ctx, arg)
}

func (q *querier) UpdateUserHashedPassword(ctx context.Context, arg database.UpdateUserHashedPasswordParams) error {
	user, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdatePersonal, user)
	if err != nil {
		// Admins can update passwords for other users.
		err = q.authorizeContext(ctx, policy.ActionUpdate, user)
		if err != nil {
			return err
		}
	}

	return q.db.UpdateUserHashedPassword(ctx, arg)
}

func (q *querier) UpdateUserLastSeenAt(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserLastSeenAt)(ctx, arg)
}

func (q *querier) UpdateUserLink(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
		return q.db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    arg.UserID,
			LoginType: arg.LoginType,
		})
	}
	return fetchAndQuery(q.log, q.auth, policy.ActionUpdatePersonal, fetch, q.db.UpdateUserLink)(ctx, arg)
}

func (q *querier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.UpdateUserLinkedID(ctx, arg)
}

func (q *querier) UpdateUserLoginType(ctx context.Context, arg database.UpdateUserLoginTypeParams) (database.User, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.User{}, err
	}
	return q.db.UpdateUserLoginType(ctx, arg)
}

func (q *querier) UpdateUserNotificationPreferences(ctx context.Context, arg database.UpdateUserNotificationPreferencesParams) (int64, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceNotificationPreference.WithOwner(arg.UserID.String())); err != nil {
		return -1, err
	}
	return q.db.UpdateUserNotificationPreferences(ctx, arg)
}

func (q *querier) UpdateUserProfile(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	u, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdatePersonal, u); err != nil {
		return database.User{}, err
	}
	return q.db.UpdateUserProfile(ctx, arg)
}

func (q *querier) UpdateUserQuietHoursSchedule(ctx context.Context, arg database.UpdateUserQuietHoursScheduleParams) (database.User, error) {
	u, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdatePersonal, u); err != nil {
		return database.User{}, err
	}
	return q.db.UpdateUserQuietHoursSchedule(ctx, arg)
}

// UpdateUserRoles updates the site roles of a user. The validation for this function include more than
// just a basic RBAC check.
func (q *querier) UpdateUserRoles(ctx context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
	// We need to fetch the user being updated to identify the change in roles.
	// This requires read access on the user in question, since the user is
	// returned from this function.
	user, err := fetch(q.log, q.auth, q.db.GetUserByID)(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}

	// The member role is always implied.
	impliedTypes := append(q.convertToDeploymentRoles(arg.GrantedRoles), rbac.RoleMember())
	// If the changeset is nothing, less rbac checks need to be done.
	added, removed := rbac.ChangeRoleSet(q.convertToDeploymentRoles(user.RBACRoles), impliedTypes)
	err = q.canAssignRoles(ctx, uuid.Nil, added, removed)
	if err != nil {
		return database.User{}, err
	}

	return q.db.UpdateUserRoles(ctx, arg)
}

func (q *querier) UpdateUserStatus(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserStatus)(ctx, arg)
}

func (q *querier) UpdateVolumeResourceMonitor(ctx context.Context, arg database.UpdateVolumeResourceMonitorParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceWorkspaceAgentResourceMonitor); err != nil {
		return err
	}

	return q.db.UpdateVolumeResourceMonitor(ctx, arg)
}

func (q *querier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.WorkspaceTable, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.WorkspaceTable, error) {
		w, err := q.db.GetWorkspaceByID(ctx, arg.ID)
		if err != nil {
			return database.WorkspaceTable{}, err
		}
		return w.WorkspaceTable(), nil
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspace)(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAgentConnectionByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentLifecycleStateByID(ctx context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, arg.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, policy.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentLogOverflowByID(ctx context.Context, arg database.UpdateWorkspaceAgentLogOverflowByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, policy.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentLogOverflowByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentMetadata(ctx context.Context, arg database.UpdateWorkspaceAgentMetadataParams) error {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, arg.WorkspaceAgentID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace)
	if err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentMetadata(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentStartupByID(ctx context.Context, arg database.UpdateWorkspaceAgentStartupByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, policy.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentStartupByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	workspace, err := q.db.GetWorkspaceByWorkspaceAppID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAppHealthByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAutomaticUpdates(ctx context.Context, arg database.UpdateWorkspaceAutomaticUpdatesParams) error {
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAutomaticUpdates(ctx, arg)
}

func (q *querier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAutostart)(ctx, arg)
}

// UpdateWorkspaceBuildCostByID is used by the provisioning system to update the cost of a workspace build.
func (q *querier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateWorkspaceBuildCostByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceBuildDeadlineByID(ctx context.Context, arg database.UpdateWorkspaceBuildDeadlineByIDParams) error {
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceBuildDeadlineByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceBuildProvisionerStateByID(ctx context.Context, arg database.UpdateWorkspaceBuildProvisionerStateByIDParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateWorkspaceBuildProvisionerStateByID(ctx, arg)
}

// Deprecated: Use SoftDeleteWorkspaceByID
func (q *querier) UpdateWorkspaceDeletedByID(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	// TODO deleteQ me, placeholder for database.Store
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	// This function is always used to deleteQ.
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateWorkspaceDeletedByID)(ctx, arg)
}

func (q *querier) UpdateWorkspaceDormantDeletingAt(ctx context.Context, arg database.UpdateWorkspaceDormantDeletingAtParams) (database.WorkspaceTable, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceDormantDeletingAtParams) (database.WorkspaceTable, error) {
		w, err := q.db.GetWorkspaceByID(ctx, arg.ID)
		if err != nil {
			return database.WorkspaceTable{}, err
		}
		return w.WorkspaceTable(), nil
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspaceDormantDeletingAt)(ctx, arg)
}

func (q *querier) UpdateWorkspaceLastUsedAt(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceLastUsedAt)(ctx, arg)
}

func (q *querier) UpdateWorkspaceNextStartAt(ctx context.Context, arg database.UpdateWorkspaceNextStartAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceNextStartAtParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceNextStartAt)(ctx, arg)
}

func (q *querier) UpdateWorkspaceProxy(ctx context.Context, arg database.UpdateWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceProxyParams) (database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxyByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspaceProxy)(ctx, arg)
}

func (q *querier) UpdateWorkspaceProxyDeleted(ctx context.Context, arg database.UpdateWorkspaceProxyDeletedParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceProxyDeletedParams) (database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxyByID(ctx, arg.ID)
	}
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateWorkspaceProxyDeleted)(ctx, arg)
}

func (q *querier) UpdateWorkspaceTTL(ctx context.Context, arg database.UpdateWorkspaceTTLParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceTTLParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceTTL)(ctx, arg)
}

func (q *querier) UpdateWorkspacesDormantDeletingAtByTemplateID(ctx context.Context, arg database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams) ([]database.WorkspaceTable, error) {
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return nil, xerrors.Errorf("get template by id: %w", err)
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return nil, err
	}
	return q.db.UpdateWorkspacesDormantDeletingAtByTemplateID(ctx, arg)
}

func (q *querier) UpdateWorkspacesTTLByTemplateID(ctx context.Context, arg database.UpdateWorkspacesTTLByTemplateIDParams) error {
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return xerrors.Errorf("get template by id: %w", err)
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return err
	}
	return q.db.UpdateWorkspacesTTLByTemplateID(ctx, arg)
}

func (q *querier) UpsertAnnouncementBanners(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertAnnouncementBanners(ctx, value)
}

func (q *querier) UpsertAppSecurityKey(ctx context.Context, data string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertAppSecurityKey(ctx, data)
}

func (q *querier) UpsertApplicationName(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertApplicationName(ctx, value)
}

func (q *querier) UpsertCoordinatorResumeTokenSigningKey(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertCoordinatorResumeTokenSigningKey(ctx, value)
}

func (q *querier) UpsertDefaultProxy(ctx context.Context, arg database.UpsertDefaultProxyParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertDefaultProxy(ctx, arg)
}

func (q *querier) UpsertHealthSettings(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertHealthSettings(ctx, value)
}

func (q *querier) UpsertJFrogXrayScanByWorkspaceAndAgentID(ctx context.Context, arg database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams) error {
	// TODO: Having to do all this extra querying makes me a sad panda.
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace by id: %w", err)
	}

	template, err := q.db.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		return xerrors.Errorf("get template by id: %w", err)
	}

	// Only template admins should be able to write JFrog Xray scans to a workspace.
	// We don't want this to be a workspace-level permission because then users
	// could overwrite their own results.
	if err := q.authorizeContext(ctx, policy.ActionCreate, template); err != nil {
		return err
	}
	return q.db.UpsertJFrogXrayScanByWorkspaceAndAgentID(ctx, arg)
}

func (q *querier) UpsertLastUpdateCheck(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertLastUpdateCheck(ctx, value)
}

func (q *querier) UpsertLogoURL(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertLogoURL(ctx, value)
}

func (q *querier) UpsertNotificationReportGeneratorLog(ctx context.Context, arg database.UpsertNotificationReportGeneratorLogParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertNotificationReportGeneratorLog(ctx, arg)
}

func (q *querier) UpsertNotificationsSettings(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertNotificationsSettings(ctx, value)
}

func (q *querier) UpsertOAuth2GithubDefaultEligible(ctx context.Context, eligible bool) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.UpsertOAuth2GithubDefaultEligible(ctx, eligible)
}

func (q *querier) UpsertOAuthSigningKey(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertOAuthSigningKey(ctx, value)
}

func (q *querier) UpsertProvisionerDaemon(ctx context.Context, arg database.UpsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	res := rbac.ResourceProvisionerDaemon.InOrg(arg.OrganizationID)
	if arg.Tags[provisionersdk.TagScope] == provisionersdk.ScopeUser {
		res.Owner = arg.Tags[provisionersdk.TagOwner]
	}
	if err := q.authorizeContext(ctx, policy.ActionCreate, res); err != nil {
		return database.ProvisionerDaemon{}, err
	}
	return q.db.UpsertProvisionerDaemon(ctx, arg)
}

func (q *querier) UpsertRuntimeConfig(ctx context.Context, arg database.UpsertRuntimeConfigParams) error {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertRuntimeConfig(ctx, arg)
}

func (q *querier) UpsertTailnetAgent(ctx context.Context, arg database.UpsertTailnetAgentParams) (database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetAgent{}, err
	}
	return q.db.UpsertTailnetAgent(ctx, arg)
}

func (q *querier) UpsertTailnetClient(ctx context.Context, arg database.UpsertTailnetClientParams) (database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetClient{}, err
	}
	return q.db.UpsertTailnetClient(ctx, arg)
}

func (q *querier) UpsertTailnetClientSubscription(ctx context.Context, arg database.UpsertTailnetClientSubscriptionParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.UpsertTailnetClientSubscription(ctx, arg)
}

func (q *querier) UpsertTailnetCoordinator(ctx context.Context, id uuid.UUID) (database.TailnetCoordinator, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetCoordinator{}, err
	}
	return q.db.UpsertTailnetCoordinator(ctx, id)
}

func (q *querier) UpsertTailnetPeer(ctx context.Context, arg database.UpsertTailnetPeerParams) (database.TailnetPeer, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetPeer{}, err
	}
	return q.db.UpsertTailnetPeer(ctx, arg)
}

func (q *querier) UpsertTailnetTunnel(ctx context.Context, arg database.UpsertTailnetTunnelParams) (database.TailnetTunnel, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetTunnel{}, err
	}
	return q.db.UpsertTailnetTunnel(ctx, arg)
}

func (q *querier) UpsertTelemetryItem(ctx context.Context, arg database.UpsertTelemetryItemParams) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertTelemetryItem(ctx, arg)
}

func (q *querier) UpsertTemplateUsageStats(ctx context.Context) error {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertTemplateUsageStats(ctx)
}

func (q *querier) UpsertWorkspaceAgentPortShare(ctx context.Context, arg database.UpsertWorkspaceAgentPortShareParams) (database.WorkspaceAgentPortShare, error) {
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.WorkspaceAgentPortShare{}, err
	}

	err = q.authorizeContext(ctx, policy.ActionUpdate, workspace)
	if err != nil {
		return database.WorkspaceAgentPortShare{}, err
	}

	return q.db.UpsertWorkspaceAgentPortShare(ctx, arg)
}

func (q *querier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, _ rbac.PreparedAuthorized) ([]database.Template, error) {
	// TODO Delete this function, all GetTemplates should be authorized. For now just call getTemplates on the authz querier.
	return q.GetTemplatesWithFilter(ctx, arg)
}

func (q *querier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	// An actor is authorized to read template group roles if they are authorized to update the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateGroupRoles(ctx, id)
}

func (q *querier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	// An actor is authorized to query template user roles if they are authorized to update the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, policy.ActionUpdate, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateUserRoles(ctx, id)
}

func (q *querier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	// TODO Delete this function, all GetWorkspaces should be authorized. For now just call GetWorkspaces on the authz querier.
	return q.GetWorkspaces(ctx, arg)
}

func (q *querier) GetAuthorizedWorkspacesAndAgentsByOwnerID(ctx context.Context, ownerID uuid.UUID, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesAndAgentsByOwnerIDRow, error) {
	return q.GetWorkspacesAndAgentsByOwnerID(ctx, ownerID)
}

// GetAuthorizedUsers is not required for dbauthz since GetUsers is already
// authenticated.
func (q *querier) GetAuthorizedUsers(ctx context.Context, arg database.GetUsersParams, _ rbac.PreparedAuthorized) ([]database.GetUsersRow, error) {
	// GetUsers is authenticated.
	return q.GetUsers(ctx, arg)
}

func (q *querier) GetAuthorizedAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams, _ rbac.PreparedAuthorized) ([]database.GetAuditLogsOffsetRow, error) {
	return q.GetAuditLogsOffset(ctx, arg)
}
