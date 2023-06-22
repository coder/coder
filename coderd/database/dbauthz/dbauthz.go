package dbauthz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/topdown"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

var _ database.Store = (*querier)(nil)

const wrapname = "dbauthz.querier"

// NoActorError wraps ErrNoRows for the api to return a 404. This is the correct
// response when the user is not authorized.
var NoActorError = xerrors.Errorf("no authorization actor in context: %w", sql.ErrNoRows)

// NotAuthorizedError is a sentinel error that unwraps to sql.ErrNoRows.
// This allows the internal error to be read by the caller if needed. Otherwise
// it will be handled as a 404.
type NotAuthorizedError struct {
	Err error
}

func (e NotAuthorizedError) Error() string {
	return fmt.Sprintf("unauthorized: %s", e.Err.Error())
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
}

func New(db database.Store, authorizer rbac.Authorizer, logger slog.Logger) database.Store {
	// If the underlying db store is already a querier, return it.
	// Do not double wrap.
	if slices.Contains(db.Wrappers(), wrapname) {
		return db
	}
	return &querier{
		db:   db,
		auth: authorizer,
		log:  logger,
	}
}

func (q *querier) Wrappers() []string {
	return append(q.db.Wrappers(), wrapname)
}

// authorizeContext is a helper function to authorize an action on an object.
func (q *querier) authorizeContext(ctx context.Context, action rbac.Action, object rbac.Objecter) error {
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
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "provisionerd",
				DisplayName: "Provisioner Daemon",
				Site: rbac.Permissions(map[string][]rbac.Action{
					// TODO: Add ProvisionerJob resource type.
					rbac.ResourceFile.Type:      {rbac.ActionRead},
					rbac.ResourceSystem.Type:    {rbac.WildcardSymbol},
					rbac.ResourceTemplate.Type:  {rbac.ActionRead, rbac.ActionUpdate},
					rbac.ResourceUser.Type:      {rbac.ActionRead},
					rbac.ResourceWorkspace.Type: {rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
					rbac.ResourceUserData.Type:  {rbac.ActionRead, rbac.ActionUpdate},
					rbac.ResourceAPIKey.Type:    {rbac.WildcardSymbol},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectAutostart = rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "autostart",
				DisplayName: "Autostart Daemon",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceSystem.Type:    {rbac.WildcardSymbol},
					rbac.ResourceTemplate.Type:  {rbac.ActionRead, rbac.ActionUpdate},
					rbac.ResourceWorkspace.Type: {rbac.ActionRead, rbac.ActionUpdate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()

	subjectSystemRestricted = rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "system",
				DisplayName: "Coder",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceWildcard.Type:           {rbac.ActionRead},
					rbac.ResourceAPIKey.Type:             {rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
					rbac.ResourceGroup.Type:              {rbac.ActionCreate, rbac.ActionUpdate},
					rbac.ResourceRoleAssignment.Type:     {rbac.ActionCreate},
					rbac.ResourceSystem.Type:             {rbac.WildcardSymbol},
					rbac.ResourceOrganization.Type:       {rbac.ActionCreate},
					rbac.ResourceOrganizationMember.Type: {rbac.ActionCreate},
					rbac.ResourceOrgRoleAssignment.Type:  {rbac.ActionCreate},
					rbac.ResourceUser.Type:               {rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
					rbac.ResourceUserData.Type:           {rbac.ActionCreate, rbac.ActionUpdate},
					rbac.ResourceWorkspace.Type:          {rbac.ActionUpdate},
					rbac.ResourceWorkspaceExecution.Type: {rbac.ActionCreate},
					rbac.ResourceWorkspaceProxy.Type:     {rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
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

// AsSystemRestricted returns a context with an actor that has permissions
// required for various system operations (login, logout, metrics cache).
func AsSystemRestricted(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, subjectSystemRestricted)
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

// insert runs an rbac.ActionCreate on the rbac object argument before
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
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, rbac.ActionCreate, object.RBACObject())
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
		rbac.ActionDelete, fetchFunc, deleteFunc)
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
	return fetchAndQuery(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateQuery)
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
	return fetchAndExec(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateExec)
}

// fetch is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
func fetch[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
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
		err = authorizer.Authorize(ctx, act, rbac.ActionRead, object.RBACObject())
		if err != nil {
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		return object, nil
	}
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
	action rbac.Action,
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
	action rbac.Action,
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
		return rbac.Filter(ctx, authorizer, act, rbac.ActionRead, objects)
	}
}

// prepareSQLFilter is a helper function that prepares a SQL filter using the
// given authorization context.
func prepareSQLFilter(ctx context.Context, authorizer rbac.Authorizer, action rbac.Action, resourceType string) (rbac.PreparedAuthorized, error) {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return nil, NoActorError
	}

	return authorizer.Prepare(ctx, act, action, resourceType)
}

func (q *querier) Ping(ctx context.Context) (time.Duration, error) {
	return q.db.Ping(ctx)
}

// InTx runs the given function in a transaction.
func (q *querier) InTx(function func(querier database.Store) error, txOpts *sql.TxOptions) error {
	return q.db.InTx(func(tx database.Store) error {
		// Wrap the transaction store in a querier.
		wrapped := New(tx, q.auth, q.log)
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
		if err := q.authorizeContext(ctx, rbac.ActionUpdate, tpl); err == nil {
			return nil
		}
	}

	return NotAuthorizedError{
		Err: xerrors.Errorf("not authorized to read file %s", file.ID),
	}
}

func (q *querier) canAssignRoles(ctx context.Context, orgID *uuid.UUID, added, removed []string) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	roleAssign := rbac.ResourceRoleAssignment
	shouldBeOrgRoles := false
	if orgID != nil {
		roleAssign = roleAssign.InOrg(*orgID)
		shouldBeOrgRoles = true
	}

	grantedRoles := append(added, removed...)
	// Validate that the roles being assigned are valid.
	for _, r := range grantedRoles {
		_, isOrgRole := rbac.IsOrgRole(r)
		if shouldBeOrgRoles && !isOrgRole {
			return xerrors.Errorf("Must only update org roles")
		}
		if !shouldBeOrgRoles && isOrgRole {
			return xerrors.Errorf("Must only update site wide roles")
		}

		// All roles should be valid roles
		if _, err := rbac.RoleByName(r); err != nil {
			return xerrors.Errorf("%q is not a supported role", r)
		}
	}

	if len(added) > 0 {
		if err := q.authorizeContext(ctx, rbac.ActionCreate, roleAssign); err != nil {
			return err
		}
	}

	if len(removed) > 0 {
		if err := q.authorizeContext(ctx, rbac.ActionDelete, roleAssign); err != nil {
			return err
		}
	}

	for _, roleName := range grantedRoles {
		if !rbac.CanAssignRole(actor.Roles, roleName) {
			return xerrors.Errorf("not authorized to assign role %q", roleName)
		}
	}

	return nil
}

func (q *querier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, _ rbac.PreparedAuthorized) ([]database.Template, error) {
	// TODO Delete this function, all GetTemplates should be authorized. For now just call getTemplates on the authz querier.
	return q.GetTemplatesWithFilter(ctx, arg)
}

func (q *querier) SoftDeleteTemplateByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:        id,
			Deleted:   true,
			UpdatedAt: database.Now(),
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetTemplateByID, deleteF)(ctx, id)
}

func (q *querier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	// An actor is authorized to read template group roles if they are authorized to read the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateGroupRoles(ctx, id)
}

func (q *querier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	// An actor is authorized to query template user roles if they are authorized to read the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateUserRoles(ctx, id)
}

func (q *querier) GetAuthorizedUserCount(ctx context.Context, arg database.GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	return q.db.GetAuthorizedUserCount(ctx, arg, prepared)
}

func (q *querier) GetUsersWithCount(ctx context.Context, arg database.GetUsersParams) ([]database.User, int64, error) {
	// TODO Implement this with a SQL filter. The count is incorrect without it.
	rowUsers, err := q.db.GetUsers(ctx, arg)
	if err != nil {
		return nil, -1, err
	}

	if len(rowUsers) == 0 {
		return []database.User{}, 0, nil
	}

	act, ok := ActorFromContext(ctx)
	if !ok {
		return nil, -1, NoActorError
	}

	// TODO: Is this correct? Should we return a restricted user?
	users := database.ConvertUserRows(rowUsers)
	users, err = rbac.Filter(ctx, q.auth, act, rbac.ActionRead, users)
	if err != nil {
		return nil, -1, err
	}

	return users, rowUsers[0].Count, nil
}

func (q *querier) SoftDeleteUserByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateUserDeletedByID(ctx, database.UpdateUserDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetUserByID, deleteF)(ctx, id)
}

func (q *querier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	// TODO Delete this function, all GetWorkspaces should be authorized. For now just call GetWorkspaces on the authz querier.
	return q.GetWorkspaces(ctx, arg)
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

func (q *querier) AcquireLock(ctx context.Context, id int64) error {
	return q.db.AcquireLock(ctx, id)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) AcquireProvisionerJob(ctx context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.AcquireProvisionerJob(ctx, arg)
}

func (q *querier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return deleteQ(q.log, q.auth, q.db.GetAPIKeyByID, q.db.DeleteAPIKeyByID)(ctx, id)
}

func (q *querier) DeleteAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	// TODO: This is not 100% correct because it omits apikey IDs.
	err := q.authorizeContext(ctx, rbac.ActionDelete,
		rbac.ResourceAPIKey.WithOwner(userID.String()))
	if err != nil {
		return err
	}
	return q.db.DeleteAPIKeysByUserID(ctx, userID)
}

func (q *querier) DeleteApplicationConnectAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	// TODO: This is not 100% correct because it omits apikey IDs.
	err := q.authorizeContext(ctx, rbac.ActionDelete,
		rbac.ResourceAPIKey.WithOwner(userID.String()))
	if err != nil {
		return err
	}
	return q.db.DeleteApplicationConnectAPIKeysByUserID(ctx, userID)
}

func (q *querier) DeleteCoordinator(ctx context.Context, id uuid.UUID) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return err
	}
	return q.db.DeleteCoordinator(ctx, id)
}

func (q *querier) DeleteGitSSHKey(ctx context.Context, userID uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetGitSSHKey, q.db.DeleteGitSSHKey)(ctx, userID)
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

func (q *querier) DeleteGroupMembersByOrgAndUser(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	// This will remove the user from all groups in the org. This counts as updating a group.
	// NOTE: instead of fetching all groups in the org with arg.UserID as a member, we instead
	// check if the caller has permission to update any group in the org.
	fetch := func(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return update(q.log, q.auth, fetch, q.db.DeleteGroupMembersByOrgAndUser)(ctx, arg)
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

func (q *querier) DeleteOldWorkspaceAgentStartupLogs(ctx context.Context) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentStartupLogs(ctx)
}

func (q *querier) DeleteOldWorkspaceAgentStats(ctx context.Context) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteOldWorkspaceAgentStats(ctx)
}

func (q *querier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *querier) DeleteTailnetAgent(ctx context.Context, arg database.DeleteTailnetAgentParams) (database.DeleteTailnetAgentRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetAgentRow{}, err
	}
	return q.db.DeleteTailnetAgent(ctx, arg)
}

func (q *querier) DeleteTailnetClient(ctx context.Context, arg database.DeleteTailnetClientParams) (database.DeleteTailnetClientRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionDelete, rbac.ResourceTailnetCoordinator); err != nil {
		return database.DeleteTailnetClientRow{}, err
	}
	return q.db.DeleteTailnetClient(ctx, arg)
}

func (q *querier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByID)(ctx, id)
}

func (q *querier) GetAPIKeyByName(ctx context.Context, arg database.GetAPIKeyByNameParams) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByName)(ctx, arg)
}

func (q *querier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *querier) GetAPIKeysByUserID(ctx context.Context, params database.GetAPIKeysByUserIDParams) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysByUserID)(ctx, database.GetAPIKeysByUserIDParams{LoginType: params.LoginType, UserID: params.UserID})
}

func (q *querier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *querier) GetActiveUserCount(ctx context.Context) (int64, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetActiveUserCount(ctx)
}

func (q *querier) GetAppSecurityKey(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetAppSecurityKey(ctx)
}

func (q *querier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	// To optimize audit logs, we only check the global audit log permission once.
	// This is because we expect a large unbounded set of audit logs, and applying a SQL
	// filter would slow down the query for no benefit.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceAuditLog); err != nil {
		return nil, err
	}
	return q.db.GetAuditLogsOffset(ctx, arg)
}

func (q *querier) GetAuthorizationUserRoles(ctx context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.GetAuthorizationUserRolesRow{}, err
	}
	return q.db.GetAuthorizationUserRoles(ctx, userID)
}

func (q *querier) GetDERPMeshKey(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetDERPMeshKey(ctx)
}

func (q *querier) GetDefaultProxyConfig(ctx context.Context) (database.GetDefaultProxyConfigRow, error) {
	// No authz checks
	return q.db.GetDefaultProxyConfig(ctx)
}

// Only used by metrics cache.
func (q *querier) GetDeploymentDAUs(ctx context.Context, tzOffset int32) ([]database.GetDeploymentDAUsRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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

func (q *querier) GetDeploymentWorkspaceStats(ctx context.Context) (database.GetDeploymentWorkspaceStatsRow, error) {
	return q.db.GetDeploymentWorkspaceStats(ctx)
}

func (q *querier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	file, err := q.db.GetFileByHashAndCreator(ctx, arg)
	if err != nil {
		return database.File{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionRead, file)
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
	err = q.authorizeContext(ctx, rbac.ActionRead, file)
	if err != nil {
		// Check the user's access to the file's templates.
		if q.authorizeUpdateFileTemplate(ctx, file) != nil {
			return database.File{}, err
		}
	}

	return file, nil
}

func (q *querier) GetFileTemplates(ctx context.Context, fileID uuid.UUID) ([]database.GetFileTemplatesRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetFileTemplates(ctx, fileID)
}

func (q *querier) GetFilteredUserCount(ctx context.Context, arg database.GetFilteredUserCountParams) (int64, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceUser.Type)
	if err != nil {
		return -1, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	// TODO: This should be the only implementation.
	return q.GetAuthorizedUserCount(ctx, arg, prep)
}

func (q *querier) GetGitAuthLink(ctx context.Context, arg database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	return fetch(q.log, q.auth, q.db.GetGitAuthLink)(ctx, arg)
}

func (q *querier) GetGitSSHKey(ctx context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	return fetch(q.log, q.auth, q.db.GetGitSSHKey)(ctx, userID)
}

func (q *querier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByID)(ctx, id)
}

func (q *querier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByOrgAndName)(ctx, arg)
}

func (q *querier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	if _, err := q.GetGroupByID(ctx, groupID); err != nil { // AuthZ check
		return nil, err
	}
	return q.db.GetGroupMembers(ctx, groupID)
}

func (q *querier) GetGroupsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]database.Group, error) {
	return fetchWithPostFilter(q.auth, q.db.GetGroupsByOrganizationID)(ctx, organizationID)
}

func (q *querier) GetLastUpdateCheck(ctx context.Context) (string, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return "", err
	}
	return q.db.GetLastUpdateCheck(ctx)
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
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetLatestWorkspaceBuilds(ctx)
}

func (q *querier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This is not ideal as not all builds will be returned if the workspace cannot be read.
	// This should probably be handled differently? Maybe join workspace builds with workspace
	// ownership properties and filter on that.
	for _, id := range ids {
		_, err := q.GetWorkspaceByID(ctx, id)
		if err != nil {
			return nil, err
		}
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
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *querier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetLogoURL(ctx)
}

func (q *querier) GetOrganizationByID(ctx context.Context, id uuid.UUID) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByID)(ctx, id)
}

func (q *querier) GetOrganizationByName(ctx context.Context, name string) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByName)(ctx, name)
}

func (q *querier) GetOrganizationIDsByMemberIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	// TODO: This should be rewritten to return a list of database.OrganizationMember for consistent RBAC objects.
	// Currently this row returns a list of org ids per user, which is challenging to check against the RBAC system.
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationIDsByMemberIDs)(ctx, ids)
}

func (q *querier) GetOrganizationMemberByUserID(ctx context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationMemberByUserID)(ctx, arg)
}

func (q *querier) GetOrganizationMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationMembershipsByUserID)(ctx, userID)
}

func (q *querier) GetOrganizations(ctx context.Context) ([]database.Organization, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.Organization, error) {
		return q.db.GetOrganizations(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *querier) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]database.Organization, error) {
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationsByUserID)(ctx, userID)
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

	err = q.authorizeContext(ctx, rbac.ActionRead, object)
	if err != nil {
		return nil, err
	}
	return q.db.GetParameterSchemasByJobID(ctx, jobID)
}

func (q *querier) GetPreviousTemplateVersion(ctx context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	// An actor can read the previous template version if they can read the related template.
	// If no linked template exists, we check if the actor can read *a* template.
	if !arg.TemplateID.Valid {
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(arg.OrganizationID)); err != nil {
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
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
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
			return database.ProvisionerJob{}, err
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		_, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return database.ProvisionerJob{}, err
		}
	default:
		return database.ProvisionerJob{}, xerrors.Errorf("unknown job type: %q", job.Type)
	}

	return job, nil
}

// TODO: we need to add a provisioner job resource
func (q *querier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
	// 	return nil, err
	// }
	return q.db.GetProvisionerJobsByIDs(ctx, ids)
}

// TODO: we need to add a provisioner job resource
func (q *querier) GetProvisionerJobsByIDsWithQueuePosition(ctx context.Context, ids []uuid.UUID) ([]database.GetProvisionerJobsByIDsWithQueuePositionRow, error) {
	return q.db.GetProvisionerJobsByIDsWithQueuePosition(ctx, ids)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) GetProvisionerJobsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.GetProvisionerJobsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetProvisionerLogsAfterID(ctx context.Context, arg database.GetProvisionerLogsAfterIDParams) ([]database.ProvisionerJobLog, error) {
	// Authorized read on job lets the actor also read the logs.
	_, err := q.GetProvisionerJobByID(ctx, arg.JobID)
	if err != nil {
		return nil, err
	}
	return q.db.GetProvisionerLogsAfterID(ctx, arg)
}

func (q *querier) GetQuotaAllowanceForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaAllowanceForUser(ctx, userID)
}

func (q *querier) GetQuotaConsumedForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaConsumedForUser(ctx, userID)
}

func (q *querier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetReplicasUpdatedAfter(ctx, updatedAt)
}

func (q *querier) GetServiceBanner(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetServiceBanner(ctx)
}

func (q *querier) GetTailnetAgents(ctx context.Context, id uuid.UUID) ([]database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetAgents(ctx, id)
}

func (q *querier) GetTailnetClientsForAgent(ctx context.Context, agentID uuid.UUID) ([]database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTailnetCoordinator); err != nil {
		return nil, err
	}
	return q.db.GetTailnetClientsForAgent(ctx, agentID)
}

// Only used by metrics cache.
func (q *querier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplateDAUs(ctx, arg)
}

func (q *querier) GetTemplateVersionByID(ctx context.Context, tvid uuid.UUID) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, tvid)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
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
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
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
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
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

	if err := q.authorizeContext(ctx, rbac.ActionRead, object); err != nil {
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

	if err := q.authorizeContext(ctx, rbac.ActionRead, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionVariables(ctx, templateVersionID)
}

// GetTemplateVersionsByIDs is only used for workspace build data.
// The workspace is already fetched.
func (q *querier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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

	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}

	return q.db.GetTemplateVersionsByTemplateID(ctx, arg)
}

func (q *querier) GetTemplateVersionsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.TemplateVersion, error) {
	// An actor can read execute this query if they can read all templates.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.All()); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetTemplates(ctx context.Context) ([]database.Template, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetTemplates(ctx)
}

func (q *querier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedTemplates(ctx, arg, prep)
}

func (q *querier) GetUnexpiredLicenses(ctx context.Context) ([]database.License, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetUnexpiredLicenses(ctx)
}

func (q *querier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByEmailOrUsername)(ctx, arg)
}

func (q *querier) GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByID)(ctx, id)
}

func (q *querier) GetUserCount(ctx context.Context) (int64, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return 0, err
	}
	return q.db.GetUserCount(ctx)
}

func (q *querier) GetUserLinkByLinkedID(ctx context.Context, linkedID string) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByLinkedID(ctx, linkedID)
}

func (q *querier) GetUserLinkByUserIDLoginType(ctx context.Context, arg database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.GetUserLinkByUserIDLoginType(ctx, arg)
}

func (q *querier) GetUsers(ctx context.Context, arg database.GetUsersParams) ([]database.GetUsersRow, error) {
	// TODO: We should use GetUsersWithCount with a better method signature.
	return fetchWithPostFilter(q.auth, q.db.GetUsers)(ctx, arg)
}

// GetUsersByIDs is only used for usernames on workspace return data.
// This function should be replaced by joining this data to the workspace query
// itself.
func (q *querier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	for _, uid := range ids {
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(uid)); err != nil {
			return nil, err
		}
	}
	return q.db.GetUsersByIDs(ctx, ids)
}

// GetWorkspaceAgentByAuthToken is used in http middleware to get the workspace agent.
// This should only be used by a system user in that middleware.
func (q *querier) GetWorkspaceAgentByAuthToken(ctx context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.GetWorkspaceAgentByAuthToken(ctx, authToken)
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

func (q *querier) GetWorkspaceAgentMetadata(ctx context.Context, workspaceAgentID uuid.UUID) ([]database.WorkspaceAgentMetadatum, error) {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, workspaceAgentID)
	if err != nil {
		return nil, err
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, workspace)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceAgentMetadata(ctx, workspaceAgentID)
}

func (q *querier) GetWorkspaceAgentStartupLogsAfter(ctx context.Context, arg database.GetWorkspaceAgentStartupLogsAfterParams) ([]database.WorkspaceAgentStartupLog, error) {
	_, err := q.GetWorkspaceAgentByID(ctx, arg.AgentID)
	if err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentStartupLogsAfter(ctx, arg)
}

func (q *querier) GetWorkspaceAgentStats(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsRow, error) {
	return q.db.GetWorkspaceAgentStats(ctx, createdAfter)
}

func (q *querier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	return q.db.GetWorkspaceAgentStatsAndLabels(ctx, createdAfter)
}

// GetWorkspaceAgentsByResourceIDs
// The workspace/job is already fetched.
func (q *querier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAppsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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

func (q *querier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, arg.WorkspaceID); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildsByWorkspaceID(ctx, arg)
}

// Telemetry related functions. These functions are system functions for returning
// telemetry data. Never called by a user.

func (q *querier) GetWorkspaceBuildsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceBuild, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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

func (q *querier) GetWorkspaceProxies(ctx context.Context) ([]database.WorkspaceProxy, error) {
	return fetchWithPostFilter(q.auth, func(ctx context.Context, _ interface{}) ([]database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxies(ctx)
	})(ctx, nil)
}

func (q *querier) GetWorkspaceProxyByHostname(ctx context.Context, params database.GetWorkspaceProxyByHostnameParams) (database.WorkspaceProxy, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
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

	if err := q.authorizeContext(ctx, rbac.ActionRead, obj); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobID(ctx, jobID)
}

// GetWorkspaceResourcesByJobIDs is only used for workspace build data.
// The workspace is already fetched.
// TODO: Find a way to replace this with proper authz.
func (q *querier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *querier) GetWorkspaceResourcesCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesCreatedAfter(ctx, createdAt)
}

func (q *querier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedWorkspaces(ctx, arg, prep)
}

func (q *querier) GetWorkspacesEligibleForTransition(ctx context.Context, now time.Time) ([]database.Workspace, error) {
	return q.db.GetWorkspacesEligibleForTransition(ctx, now)
}

func (q *querier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return insert(q.log, q.auth,
		rbac.ResourceAPIKey.WithOwner(arg.UserID.String()),
		q.db.InsertAPIKey)(ctx, arg)
}

func (q *querier) InsertAllUsersGroup(ctx context.Context, organizationID uuid.UUID) (database.Group, error) {
	// This method creates a new group.
	return insert(q.log, q.auth, rbac.ResourceGroup.InOrg(organizationID), q.db.InsertAllUsersGroup)(ctx, organizationID)
}

func (q *querier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	return insert(q.log, q.auth, rbac.ResourceAuditLog, q.db.InsertAuditLog)(ctx, arg)
}

func (q *querier) InsertDERPMeshKey(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDERPMeshKey(ctx, value)
}

func (q *querier) InsertDeploymentID(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.InsertDeploymentID(ctx, value)
}

func (q *querier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	return insert(q.log, q.auth, rbac.ResourceFile.WithOwner(arg.CreatedBy.String()), q.db.InsertFile)(ctx, arg)
}

func (q *querier) InsertGitAuthLink(ctx context.Context, arg database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	return insert(q.log, q.auth, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.db.InsertGitAuthLink)(ctx, arg)
}

func (q *querier) InsertGitSSHKey(ctx context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	return insert(q.log, q.auth, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.db.InsertGitSSHKey)(ctx, arg)
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

func (q *querier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceLicense); err != nil {
		return database.License{}, err
	}
	return q.db.InsertLicense(ctx, arg)
}

func (q *querier) InsertOrganization(ctx context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	return insert(q.log, q.auth, rbac.ResourceOrganization, q.db.InsertOrganization)(ctx, arg)
}

func (q *querier) InsertOrganizationMember(ctx context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	// All roles are added roles. Org member is always implied.
	addedRoles := append(arg.Roles, rbac.RoleOrgMember(arg.OrganizationID))
	err := q.canAssignRoles(ctx, &arg.OrganizationID, addedRoles, []string{})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	obj := rbac.ResourceOrganizationMember.InOrg(arg.OrganizationID).WithID(arg.UserID)
	return insert(q.log, q.auth, obj, q.db.InsertOrganizationMember)(ctx, arg)
}

// TODO: We need to create a ProvisionerDaemon resource type
func (q *querier) InsertProvisionerDaemon(ctx context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerDaemon{}, err
	// }
	return q.db.InsertProvisionerDaemon(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJob(ctx context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return database.ProvisionerJob{}, err
	// }
	return q.db.InsertProvisionerJob(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) InsertProvisionerJobLogs(ctx context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	// if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
	// return nil, err
	// }
	return q.db.InsertProvisionerJobLogs(ctx, arg)
}

func (q *querier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.InsertReplica(ctx, arg)
}

func (q *querier) InsertTemplate(ctx context.Context, arg database.InsertTemplateParams) (database.Template, error) {
	obj := rbac.ResourceTemplate.InOrg(arg.OrganizationID)
	return insert(q.log, q.auth, obj, q.db.InsertTemplate)(ctx, arg)
}

func (q *querier) InsertTemplateVersion(ctx context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	if !arg.TemplateID.Valid {
		// Making a new template version is the same permission as creating a new template.
		err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(arg.OrganizationID))
		if err != nil {
			return database.TemplateVersion{}, err
		}
	} else {
		// Must do an authorized fetch to prevent leaking template ids this way.
		tpl, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		// Check the create permission on the template.
		err = q.authorizeContext(ctx, rbac.ActionCreate, tpl)
		if err != nil {
			return database.TemplateVersion{}, err
		}
	}

	return q.db.InsertTemplateVersion(ctx, arg)
}

func (q *querier) InsertTemplateVersionParameter(ctx context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionParameter{}, err
	}
	return q.db.InsertTemplateVersionParameter(ctx, arg)
}

func (q *querier) InsertTemplateVersionVariable(ctx context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.TemplateVersionVariable{}, err
	}
	return q.db.InsertTemplateVersionVariable(ctx, arg)
}

func (q *querier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	// Always check if the assigned roles can actually be assigned by this actor.
	impliedRoles := append([]string{rbac.RoleMember()}, arg.RBACRoles...)
	err := q.canAssignRoles(ctx, nil, impliedRoles, []string{})
	if err != nil {
		return database.User{}, err
	}
	obj := rbac.ResourceUser
	return insert(q.log, q.auth, obj, q.db.InsertUser)(ctx, arg)
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
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceUser.WithID(arg.UserID)); err != nil {
		return database.UserLink{}, err
	}
	return q.db.InsertUserLink(ctx, arg)
}

func (q *querier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	return insert(q.log, q.auth, obj, q.db.InsertWorkspace)(ctx, arg)
}

// Provisionerd server functions

func (q *querier) InsertWorkspaceAgent(ctx context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.InsertWorkspaceAgent(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentMetadata(ctx context.Context, arg database.InsertWorkspaceAgentMetadataParams) error {
	// We don't check for workspace ownership here since the agent metadata may
	// be associated with an orphaned agent used by a dry run build.
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return err
	}

	return q.db.InsertWorkspaceAgentMetadata(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentStartupLogs(ctx context.Context, arg database.InsertWorkspaceAgentStartupLogsParams) ([]database.WorkspaceAgentStartupLog, error) {
	return q.db.InsertWorkspaceAgentStartupLogs(ctx, arg)
}

func (q *querier) InsertWorkspaceAgentStat(ctx context.Context, arg database.InsertWorkspaceAgentStatParams) (database.WorkspaceAgentStat, error) {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	// Not really sure what this is for.
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.WorkspaceAgentStat{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return database.WorkspaceAgentStat{}, err
	}
	return q.db.InsertWorkspaceAgentStat(ctx, arg)
}

func (q *querier) InsertWorkspaceApp(ctx context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceApp{}, err
	}
	return q.db.InsertWorkspaceApp(ctx, arg)
}

func (q *querier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	w, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	var action rbac.Action = rbac.ActionUpdate
	if arg.Transition == database.WorkspaceTransitionDelete {
		action = rbac.ActionDelete
	}

	if err = q.authorizeContext(ctx, action, w); err != nil {
		return database.WorkspaceBuild{}, err
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

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return err
	}

	return q.db.InsertWorkspaceBuildParameters(ctx, arg)
}

func (q *querier) InsertWorkspaceProxy(ctx context.Context, arg database.InsertWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	return insert(q.log, q.auth, rbac.ResourceWorkspaceProxy, q.db.InsertWorkspaceProxy)(ctx, arg)
}

func (q *querier) InsertWorkspaceResource(ctx context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceResource{}, err
	}
	return q.db.InsertWorkspaceResource(ctx, arg)
}

func (q *querier) InsertWorkspaceResourceMetadata(ctx context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceSystem); err != nil {
		return nil, err
	}
	return q.db.InsertWorkspaceResourceMetadata(ctx, arg)
}

func (q *querier) RegisterWorkspaceProxy(ctx context.Context, arg database.RegisterWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	fetch := func(ctx context.Context, arg database.RegisterWorkspaceProxyParams) (database.WorkspaceProxy, error) {
		return q.db.GetWorkspaceProxyByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.RegisterWorkspaceProxy)(ctx, arg)
}

func (q *querier) TryAcquireLock(ctx context.Context, id int64) (bool, error) {
	return q.db.TryAcquireLock(ctx, id)
}

func (q *querier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.db.GetAPIKeyByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateAPIKeyByID)(ctx, arg)
}

func (q *querier) UpdateGitAuthLink(ctx context.Context, arg database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
	fetch := func(ctx context.Context, arg database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
		return q.db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{UserID: arg.UserID, ProviderID: arg.ProviderID})
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGitAuthLink)(ctx, arg)
}

func (q *querier) UpdateGitSSHKey(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	fetch := func(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
		return q.db.GetGitSSHKey(ctx, arg.UserID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGitSSHKey)(ctx, arg)
}

func (q *querier) UpdateGroupByID(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	fetch := func(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGroupByID)(ctx, arg)
}

func (q *querier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// Authorized fetch will check that the actor has read access to the org member since the org member is returned.
	member, err := q.GetOrganizationMemberByUserID(ctx, database.GetOrganizationMemberByUserIDParams{
		OrganizationID: arg.OrgID,
		UserID:         arg.UserID,
	})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	// The org member role is always implied.
	impliedTypes := append(arg.GrantedRoles, rbac.RoleOrgMember(arg.OrgID))
	added, removed := rbac.ChangeRoleSet(member.Roles, impliedTypes)
	err = q.canAssignRoles(ctx, &arg.OrgID, added, removed)
	if err != nil {
		return database.OrganizationMember{}, err
	}

	return q.db.UpdateMemberRoles(ctx, arg)
}

// TODO: We need to create a ProvisionerJob resource type
func (q *querier) UpdateProvisionerJobByID(ctx context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
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

		err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
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
			err = q.authorizeContext(ctx, rbac.ActionUpdate, templateVersion.RBACObject(template))
			if err != nil {
				return err
			}
		} else {
			err = q.authorizeContext(ctx, rbac.ActionUpdate, templateVersion.RBACObjectNoTemplate())
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
	// if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
	// return err
	// }
	return q.db.UpdateProvisionerJobWithCompleteByID(ctx, arg)
}

func (q *querier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.Replica{}, err
	}
	return q.db.UpdateReplica(ctx, arg)
}

func (q *querier) UpdateTemplateACLByID(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
	// UpdateTemplateACL uses the ActionCreate action. Only users that can create the template
	// may update the ACL.
	fetch := func(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return fetchAndQuery(q.log, q.auth, rbac.ActionCreate, fetch, q.db.UpdateTemplateACLByID)(ctx, arg)
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

func (q *querier) UpdateTemplateMetaByID(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
	fetch := func(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateTemplateMetaByID)(ctx, arg)
}

func (q *querier) UpdateTemplateScheduleByID(ctx context.Context, arg database.UpdateTemplateScheduleByIDParams) (database.Template, error) {
	fetch := func(ctx context.Context, arg database.UpdateTemplateScheduleByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateTemplateScheduleByID)(ctx, arg)
}

func (q *querier) UpdateTemplateVersionByID(ctx context.Context, arg database.UpdateTemplateVersionByIDParams) (database.TemplateVersion, error) {
	// An actor is allowed to update the template version if they are authorized to update the template.
	tv, err := q.db.GetTemplateVersionByID(ctx, arg.ID)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	var obj rbac.Objecter
	if !tv.TemplateID.Valid {
		obj = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		tpl, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		obj = tpl
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, obj); err != nil {
		return database.TemplateVersion{}, err
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
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionDescriptionByJobID(ctx, arg)
}

func (q *querier) UpdateTemplateVersionGitAuthProvidersByJobID(ctx context.Context, arg database.UpdateTemplateVersionGitAuthProvidersByJobIDParams) error {
	// An actor is allowed to update the template version git auth providers if they are authorized to update the template.
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
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionGitAuthProvidersByJobID(ctx, arg)
}

// UpdateUserDeletedByID
// Deprecated: Delete this function in favor of 'SoftDeleteUserByID'. Deletes are
// irreversible.
func (q *querier) UpdateUserDeletedByID(ctx context.Context, arg database.UpdateUserDeletedByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateUserDeletedByIDParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	// This uses the rbac.ActionDelete action always as this function should always delete.
	// We should delete this function in favor of 'SoftDeleteUserByID'.
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateUserDeletedByID)(ctx, arg)
}

func (q *querier) UpdateUserHashedPassword(ctx context.Context, arg database.UpdateUserHashedPasswordParams) error {
	user, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, user.UserDataRBACObject())
	if err != nil {
		// Admins can update passwords for other users.
		err = q.authorizeContext(ctx, rbac.ActionUpdate, user.RBACObject())
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
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserLink)(ctx, arg)
}

func (q *querier) UpdateUserLinkedID(ctx context.Context, arg database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.UserLink{}, err
	}
	return q.db.UpdateUserLinkedID(ctx, arg)
}

func (q *querier) UpdateUserProfile(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	u, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, u.UserDataRBACObject()); err != nil {
		return database.User{}, err
	}
	return q.db.UpdateUserProfile(ctx, arg)
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
	impliedTypes := append(arg.GrantedRoles, rbac.RoleMember())
	// If the changeset is nothing, less rbac checks need to be done.
	added, removed := rbac.ChangeRoleSet(user.RBACRoles, impliedTypes)
	err = q.canAssignRoles(ctx, nil, added, removed)
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

func (q *querier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspace)(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAgentConnectionByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentLifecycleStateByID(ctx context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, arg.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentMetadata(ctx context.Context, arg database.UpdateWorkspaceAgentMetadataParams) error {
	workspace, err := q.db.GetWorkspaceByAgentID(ctx, arg.WorkspaceAgentID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
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

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentStartupByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentStartupLogOverflowByID(ctx context.Context, arg database.UpdateWorkspaceAgentStartupLogOverflowByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentStartupLogOverflowByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	workspace, err := q.db.GetWorkspaceByWorkspaceAppID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAppHealthByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAutostart)(ctx, arg)
}

func (q *querier) UpdateWorkspaceBuildByID(ctx context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.ID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	return q.db.UpdateWorkspaceBuildByID(ctx, arg)
}

// UpdateWorkspaceBuildCostByID is used by the provisioning system to update the cost of a workspace build.
func (q *querier) UpdateWorkspaceBuildCostByID(ctx context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.UpdateWorkspaceBuildCostByID(ctx, arg)
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

func (q *querier) UpdateWorkspaceLastUsedAt(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceLastUsedAt)(ctx, arg)
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

func (q *querier) UpdateWorkspaceTTLToBeWithinTemplateMax(ctx context.Context, arg database.UpdateWorkspaceTTLToBeWithinTemplateMaxParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceTTLToBeWithinTemplateMaxParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.TemplateID)
	}
	return fetchAndExec(q.log, q.auth, rbac.ActionUpdate, fetch, q.db.UpdateWorkspaceTTLToBeWithinTemplateMax)(ctx, arg)
}

func (q *querier) UpsertAppSecurityKey(ctx context.Context, data string) error {
	// No authz checks as this is done during startup
	return q.db.UpsertAppSecurityKey(ctx, data)
}

func (q *querier) UpsertDefaultProxy(ctx context.Context, arg database.UpsertDefaultProxyParams) error {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertDefaultProxy(ctx, arg)
}

func (q *querier) UpsertLastUpdateCheck(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceSystem); err != nil {
		return err
	}
	return q.db.UpsertLastUpdateCheck(ctx, value)
}

func (q *querier) UpsertLogoURL(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceDeploymentValues); err != nil {
		return err
	}
	return q.db.UpsertLogoURL(ctx, value)
}

func (q *querier) UpsertServiceBanner(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceDeploymentValues); err != nil {
		return err
	}
	return q.db.UpsertServiceBanner(ctx, value)
}

func (q *querier) UpsertTailnetAgent(ctx context.Context, arg database.UpsertTailnetAgentParams) (database.TailnetAgent, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetAgent{}, err
	}
	return q.db.UpsertTailnetAgent(ctx, arg)
}

func (q *querier) UpsertTailnetClient(ctx context.Context, arg database.UpsertTailnetClientParams) (database.TailnetClient, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetClient{}, err
	}
	return q.db.UpsertTailnetClient(ctx, arg)
}

func (q *querier) UpsertTailnetCoordinator(ctx context.Context, id uuid.UUID) (database.TailnetCoordinator, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceTailnetCoordinator); err != nil {
		return database.TailnetCoordinator{}, err
	}
	return q.db.UpsertTailnetCoordinator(ctx, id)
}
