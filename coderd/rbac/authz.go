package rbac

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ammario/tlru"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/util/slice"
)

// Action represents the allowed actions to be done on an object.
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// AllActions is a helper function to return all the possible actions types.
func AllActions() []Action {
	return []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}
}

type AuthCall struct {
	Actor  Subject
	Action Action
	Object Object
}

// hashAuthorizeCall guarantees a unique hash for a given auth call.
// If two hashes are equal, then the result of a given authorize() call
// will be the same.
//
// Note that this ignores some fields such as the permissions within a given
// role, as this assumes all roles are static to a given role name.
func hashAuthorizeCall(actor Subject, action Action, object Object) [32]byte {
	var hashOut [32]byte
	hash := sha256.New()

	// We use JSON for the forward security benefits if the rbac structs are
	// modified without consideration for the caching layer.
	enc := json.NewEncoder(hash)
	_ = enc.Encode(actor)
	_ = enc.Encode(action)
	_ = enc.Encode(object)

	// We might be able to avoid this extra copy?
	// sha256.Sum256() returns a [32]byte. We need to return
	// an array vs a slice so we can use it as a key in the cache.
	image := hash.Sum(nil)
	copy(hashOut[:], image)
	return hashOut
}

// Subject is a struct that contains all the elements of a subject in an rbac
// authorize.
type Subject struct {
	ID     string
	Roles  ExpandableRoles
	Groups []string
	Scope  ExpandableScope

	// cachedASTValue is the cached ast value for this subject.
	cachedASTValue ast.Value
}

// WithCachedASTValue can be called if the subject is static. This will compute
// the ast value once and cache it for future calls.
func (s Subject) WithCachedASTValue() Subject {
	tmp := s
	v, err := tmp.regoValue()
	if err == nil {
		tmp.cachedASTValue = v
	}
	return tmp
}

func (s Subject) Equal(b Subject) bool {
	if s.ID != b.ID {
		return false
	}

	if !slice.SameElements(s.Groups, b.Groups) {
		return false
	}

	if !slice.SameElements(s.SafeRoleNames(), b.SafeRoleNames()) {
		return false
	}

	if s.SafeScopeName() != b.SafeScopeName() {
		return false
	}
	return true
}

// SafeScopeName prevent nil pointer dereference.
func (s Subject) SafeScopeName() string {
	if s.Scope == nil {
		return "no-scope"
	}
	return s.Scope.Name()
}

// SafeRoleNames prevent nil pointer dereference.
func (s Subject) SafeRoleNames() []string {
	if s.Roles == nil {
		return []string{}
	}
	return s.Roles.Names()
}

type Authorizer interface {
	// Authorize will authorize the given subject to perform the given action
	// on the given object. Authorize is pure and deterministic with respect to
	// its arguments and the surrounding object.
	Authorize(ctx context.Context, subject Subject, action Action, object Object) error
	Prepare(ctx context.Context, subject Subject, action Action, objectType string) (PreparedAuthorized, error)
}

type PreparedAuthorized interface {
	Authorize(ctx context.Context, object Object) error
	CompileToSQL(ctx context.Context, cfg regosql.ConvertConfig) (string, error)
}

// Filter takes in a list of objects, and will filter the list removing all
// the elements the subject does not have permission for. All objects must be
// of the same type.
//
// Ideally the 'CompileToSQL' is used instead for large sets. This cost scales
// linearly with the number of objects passed in.
func Filter[O Objecter](ctx context.Context, auth Authorizer, subject Subject, action Action, objects []O) ([]O, error) {
	if len(objects) == 0 {
		// Nothing to filter
		return objects, nil
	}
	objectType := objects[0].RBACObject().Type
	filtered := make([]O, 0)

	// Start the span after the object type is detected. If we are filtering 0
	// objects, then the span is not interesting. It would just add excessive
	// 0 time spans that provide no insight.
	ctx, span := tracing.StartSpan(ctx,
		rbacTraceAttributes(subject, action, objectType,
			// For filtering, we are only measuring the total time for the entire
			// set of objects. This and the 'Prepare' span time
			// is all that is required to measure the performance of this
			// function on a per-object basis.
			attribute.Int("num_objects", len(objects)),
		),
	)
	defer span.End()

	// Running benchmarks on this function, it is **always** faster to call
	// auth.Authorize on <10 objects. This is because the overhead of
	// 'Prepare'. Once we cross 10 objects, then it starts to become
	// faster
	if len(objects) < 10 {
		for _, o := range objects {
			rbacObj := o.RBACObject()
			if rbacObj.Type != objectType {
				return nil, xerrors.Errorf("object types must be uniform across the set (%s), found %s", objectType, rbacObj)
			}
			err := auth.Authorize(ctx, subject, action, o.RBACObject())
			if err == nil {
				filtered = append(filtered, o)
			} else if !IsUnauthorizedError(err) {
				// If the error is not the expected "Unauthorized" error, then
				// it is something unexpected.
				return nil, err
			}
		}
		return filtered, nil
	}

	prepared, err := auth.Prepare(ctx, subject, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	for _, object := range objects {
		rbacObj := object.RBACObject()
		if rbacObj.Type != objectType {
			return nil, xerrors.Errorf("object types must be uniform across the set (%s), found %s", objectType, object.RBACObject().Type)
		}
		err := prepared.Authorize(ctx, rbacObj)
		if err == nil {
			filtered = append(filtered, object)
		} else if !IsUnauthorizedError(err) {
			// If the error is not the expected "Unauthorized" error, then
			// it is something unexpected.
			return nil, err
		}
	}

	return filtered, nil
}

// RegoAuthorizer will use a prepared rego query for performing authorize()
type RegoAuthorizer struct {
	query        rego.PreparedEvalQuery
	partialQuery rego.PreparedPartialQuery

	authorizeHist *prometheus.HistogramVec
	prepareHist   prometheus.Histogram
}

var _ Authorizer = (*RegoAuthorizer)(nil)

var (
	// Load the policy from policy.rego in this directory.
	//
	//go:embed policy.rego
	policy       string
	queryOnce    sync.Once
	query        rego.PreparedEvalQuery
	partialQuery rego.PreparedPartialQuery
)

// NewCachingAuthorizer returns a new RegoAuthorizer that supports context based
// caching. To utilize the caching, the context passed to Authorize() must be
// created with 'WithCacheCtx(ctx)'.
func NewCachingAuthorizer(registry prometheus.Registerer) Authorizer {
	return Cacher(NewAuthorizer(registry))
}

func NewAuthorizer(registry prometheus.Registerer) *RegoAuthorizer {
	queryOnce.Do(func() {
		var err error
		query, err = rego.New(
			rego.Query("data.authz.allow"),
			rego.Module("policy.rego", policy),
		).PrepareForEval(context.Background())
		if err != nil {
			panic(xerrors.Errorf("compile rego: %w", err))
		}

		partialQuery, err = rego.New(
			rego.Unknowns([]string{
				"input.object.id",
				"input.object.owner",
				"input.object.org_owner",
				"input.object.acl_user_list",
				"input.object.acl_group_list",
			}),
			rego.Query("data.authz.allow = true"),
			rego.Module("policy.rego", policy),
		).PrepareForPartial(context.Background())
		if err != nil {
			panic(xerrors.Errorf("compile partial rego: %w", err))
		}
	})

	// Register metrics to prometheus.
	// These bucket values are based on the average time it takes to run authz
	// being around 1ms. Anything under ~2ms is OK and does not need to be
	// analyzed any further.
	buckets := []float64{
		0.0005, // 0.5ms
		0.001,  // 1ms
		0.002,  // 2ms
		0.003,
		0.005,
		0.01, // 10ms
		0.02,
		0.035, // 35ms
		0.05,
		0.075,
		0.1,  // 100ms
		0.25, // 250ms
		0.75, // 750ms
		1,    // 1s
	}

	factory := promauto.With(registry)
	authorizeHistogram := factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "authz",
		Name:      "authorize_duration_seconds",
		Help:      "Duration of the 'Authorize' call in seconds. Only counts calls that succeed.",
		Buckets:   buckets,
	}, []string{"allowed"})

	prepareHistogram := factory.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "authz",
		Name:      "prepare_authorize_duration_seconds",
		Help:      "Duration of the 'PrepareAuthorize' call in seconds.",
		Buckets:   buckets,
	})

	return &RegoAuthorizer{
		query:        query,
		partialQuery: partialQuery,

		authorizeHist: authorizeHistogram,
		prepareHist:   prepareHistogram,
	}
}

type authSubject struct {
	ID     string   `json:"id"`
	Roles  []Role   `json:"roles"`
	Groups []string `json:"groups"`
	Scope  Scope    `json:"scope"`
}

// Authorize is the intended function to be used outside this package.
// It returns `nil` if the subject is authorized to perform the action on
// the object.
// If an error is returned, the authorization is denied.
func (a RegoAuthorizer) Authorize(ctx context.Context, subject Subject, action Action, object Object) error {
	start := time.Now()
	ctx, span := tracing.StartSpan(ctx,
		trace.WithTimestamp(start), // Reuse the time.Now for metric and trace
		rbacTraceAttributes(subject, action, object.Type,
			// For authorizing a single object, this data is useful to know how
			// complex our objects are getting.
			attribute.Int("object_num_groups", len(object.ACLGroupList)),
			attribute.Int("object_num_users", len(object.ACLUserList)),
		),
	)
	defer span.End()

	err := a.authorize(ctx, subject, action, object)

	span.SetAttributes(attribute.Bool("authorized", err == nil))

	dur := time.Since(start)
	if err != nil {
		a.authorizeHist.WithLabelValues("false").Observe(dur.Seconds())
		return err
	}

	a.authorizeHist.WithLabelValues("true").Observe(dur.Seconds())
	return nil
}

// authorize is the internal function that does the actual authorization.
// It is a different function so the exported one can add tracing + metrics.
// That code tends to clutter up the actual logic, so it's separated out.
// nolint:revive
func (a RegoAuthorizer) authorize(ctx context.Context, subject Subject, action Action, object Object) error {
	if subject.Roles == nil {
		return xerrors.Errorf("subject must have roles")
	}
	if subject.Scope == nil {
		return xerrors.Errorf("subject must have a scope")
	}

	astV, err := regoInputValue(subject, action, object)
	if err != nil {
		return xerrors.Errorf("convert input to value: %w", err)
	}

	results, err := a.query.Eval(ctx, rego.EvalParsedInput(astV))
	if err != nil {
		err = correctCancelError(err)
		return xerrors.Errorf("evaluate rego: %w", err)
	}

	if !results.Allowed() {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), subject, action, object, results)
	}
	return nil
}

// Prepare will partially execute the rego policy leaving the object fields unknown (except for the type).
// This will vastly speed up performance if batch authorization on the same type of objects is needed.
func (a RegoAuthorizer) Prepare(ctx context.Context, subject Subject, action Action, objectType string) (PreparedAuthorized, error) {
	start := time.Now()
	ctx, span := tracing.StartSpan(ctx,
		trace.WithTimestamp(start),
		rbacTraceAttributes(subject, action, objectType),
	)
	defer span.End()

	prepared, err := a.newPartialAuthorizer(ctx, subject, action, objectType)
	if err != nil {
		err = correctCancelError(err)
		return nil, xerrors.Errorf("new partial authorizer: %w", err)
	}

	// Add attributes of the Prepare results. This will help understand the
	// complexity of the roles and how it affects the time taken.
	span.SetAttributes(
		attribute.Int("num_queries", len(prepared.preparedQueries)),
		attribute.Bool("always_true", prepared.alwaysTrue),
	)

	a.prepareHist.Observe(time.Since(start).Seconds())
	return prepared, nil
}

// PartialAuthorizer is a prepared authorizer with the subject, action, and
// resource type fields already filled in. This speeds up authorization
// when authorizing the same type of object numerous times.
// See rbac.Filter for example usage.
type PartialAuthorizer struct {
	// partialQueries is mainly used for unit testing to assert our rego policy
	// can always be compressed into a set of queries.
	partialQueries *rego.PartialQueries

	// input is used purely for debugging and logging.
	subjectInput        Subject
	subjectAction       Action
	subjectResourceType Object

	// preparedQueries are the compiled set of queries after partial evaluation.
	// Cache these prepared queries to avoid re-compiling the queries.
	// If alwaysTrue is true, then ignore these.
	preparedQueries []rego.PreparedEvalQuery
	// alwaysTrue is if the subject can always perform the action on the
	// resource type, regardless of the unknown fields.
	alwaysTrue bool
}

var _ PreparedAuthorized = (*PartialAuthorizer)(nil)

// CompileToSQL converts the remaining rego queries into SQL WHERE clauses.
func (pa *PartialAuthorizer) CompileToSQL(ctx context.Context, cfg regosql.ConvertConfig) (string, error) {
	_, span := tracing.StartSpan(ctx, trace.WithAttributes(
		// Query count is a rough indicator of the complexity of the query
		// that needs to be converted into SQL.
		attribute.Int("query_count", len(pa.preparedQueries)),
		attribute.Bool("always_true", pa.alwaysTrue),
	))
	defer span.End()

	filter, err := Compile(cfg, pa)
	if err != nil {
		return "", xerrors.Errorf("compile: %w", err)
	}
	return filter.SQLString(), nil
}

func (pa *PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if pa.alwaysTrue {
		return nil
	}

	// If we have no queries, then no queries can return 'true'.
	// So the result is always 'false'.
	if len(pa.preparedQueries) == 0 {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"),
			pa.subjectInput, pa.subjectAction, pa.subjectResourceType, nil)
	}

	parsed, err := ast.InterfaceToValue(map[string]interface{}{
		"object": object,
	})
	if err != nil {
		return xerrors.Errorf("parse object: %w", err)
	}

	// How to interpret the results of the partial queries.
	// We have a list of queries that are along the lines of:
	// 	`input.object.org_owner = ""; "me" = input.object.owner`
	//	`input.object.org_owner in {"feda2e52-8bf1-42ce-ad75-6c5595cb297a"} `
	// All these queries are joined by an 'OR'. So we need to run through each
	// query, and evaluate it.
	//
	// In each query, we have a list of the expressions, which should be
	// all boolean expressions. In the above 1st example, there are 2.
	// These expressions within a single query are `AND` together by rego.
EachQueryLoop:
	for _, q := range pa.preparedQueries {
		// We need to eval each query with the newly known fields.
		results, err := q.Eval(ctx, rego.EvalParsedInput(parsed))
		if err != nil {
			err = correctCancelError(err)
			return xerrors.Errorf("eval error: %w", err)
		}

		// If there are no results, then the query is false. This is because rego
		// treats false queries as "undefined". So if any expression is false, the
		// result is an empty list.
		if len(results) == 0 {
			continue EachQueryLoop
		}

		// If there is more than 1 result, that means there is more than 1 rule.
		// This should not happen, because our query should always be an expression.
		// If this every occurs, it is likely the original query was not an expression.
		if len(results) > 1 {
			continue EachQueryLoop
		}

		// Our queries should be simple, and should not yield any bindings.
		// A binding is something like 'x := 1'. This binding as an expression is
		// 'true', but in our case is unhelpful. We are not analyzing this ast to
		// map bindings. So just error out. Similar to above, our queries should
		// always be boolean expressions.
		if len(results[0].Bindings) > 0 {
			continue EachQueryLoop
		}

		// We have a valid set of boolean expressions! All expressions are 'AND'd
		// together. This is automatic by rego, so we should not actually need to
		// inspect this any further. But just in case, we will verify each expression
		// did resolve to 'true'. This is purely defensive programming.
		for _, exp := range results[0].Expressions {
			if v, ok := exp.Value.(bool); !ok || !v {
				continue EachQueryLoop
			}
		}

		return nil
	}

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"),
		pa.subjectInput, pa.subjectAction, pa.subjectResourceType, nil)
}

func (a RegoAuthorizer) newPartialAuthorizer(ctx context.Context, subject Subject, action Action, objectType string) (*PartialAuthorizer, error) {
	if subject.Roles == nil {
		return nil, xerrors.Errorf("subject must have roles")
	}
	if subject.Scope == nil {
		return nil, xerrors.Errorf("subject must have a scope")
	}

	input, err := regoPartialInputValue(subject, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare input: %w", err)
	}

	partialQueries, err := a.partialQuery.Partial(ctx, rego.EvalParsedInput(input))
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	pAuth := &PartialAuthorizer{
		partialQueries:  partialQueries,
		preparedQueries: []rego.PreparedEvalQuery{},
		subjectInput:    subject,
		subjectResourceType: Object{
			Type: objectType,
			ID:   "prepared-object",
		},
		subjectAction: action,
	}

	// Prepare each query to optimize the runtime when we iterate over the objects.
	preparedQueries := make([]rego.PreparedEvalQuery, 0, len(partialQueries.Queries))
	for _, q := range partialQueries.Queries {
		if q.String() == "" {
			// No more work needed. An empty query is the same as
			//	'WHERE true'
			// This is likely an admin. We don't even need to use rego going
			// forward.
			pAuth.alwaysTrue = true
			preparedQueries = []rego.PreparedEvalQuery{}
			break
		}
		results, err := rego.New(
			rego.ParsedQuery(q),
		).PrepareForEval(ctx)
		if err != nil {
			return nil, xerrors.Errorf("prepare query %s: %w", q.String(), err)
		}
		preparedQueries = append(preparedQueries, results)
	}
	pAuth.preparedQueries = preparedQueries

	return pAuth, nil
}

// AuthorizeFilter is a compiled partial query that can be converted to SQL.
// This allows enforcing the policy on the database side in a WHERE clause.
type AuthorizeFilter interface {
	SQLString() string
}

type authorizedSQLFilter struct {
	sqlString string
	auth      *PartialAuthorizer
}

// ConfigWithACL is the basic configuration for converting rego to SQL when
// the object has group and user ACL fields.
func ConfigWithACL() regosql.ConvertConfig {
	return regosql.ConvertConfig{
		VariableConverter: regosql.DefaultVariableConverter(),
	}
}

// ConfigWithoutACL is the basic configuration for converting rego to SQL when
// the object has no ACL fields.
func ConfigWithoutACL() regosql.ConvertConfig {
	return regosql.ConvertConfig{
		VariableConverter: regosql.NoACLConverter(),
	}
}

func ConfigWorkspaces() regosql.ConvertConfig {
	return regosql.ConvertConfig{
		VariableConverter: regosql.WorkspaceConverter(),
	}
}

func Compile(cfg regosql.ConvertConfig, pa *PartialAuthorizer) (AuthorizeFilter, error) {
	root, err := regosql.ConvertRegoAst(cfg, pa.partialQueries)
	if err != nil {
		return nil, xerrors.Errorf("convert rego ast: %w", err)
	}

	// Generate the SQL
	gen := sqltypes.NewSQLGenerator()
	sqlString := root.SQLString(gen)
	if len(gen.Errors()) > 0 {
		var errStrings []string
		for _, err := range gen.Errors() {
			errStrings = append(errStrings, err.Error())
		}
		return nil, xerrors.Errorf("sql generation errors: %v", strings.Join(errStrings, ", "))
	}

	return &authorizedSQLFilter{
		sqlString: sqlString,
		auth:      pa,
	}, nil
}

func (a *authorizedSQLFilter) SQLString() string {
	return a.sqlString
}

type authCache struct {
	// cache is a cache of hashed Authorize inputs to the result of the Authorize
	// call.
	// determistic function.
	cache *tlru.Cache[[32]byte, error]

	authz Authorizer
}

// Cacher returns an Authorizer that can use a cache to short circuit duplicate
// calls to the Authorizer. This is useful when multiple calls are made to the
// Authorizer for the same subject, action, and object.
// This is a GLOBAL cache shared between all requests.
// If no cache is found on the context, the Authorizer is called as normal.
//
// Cacher is safe for multiple actors.
func Cacher(authz Authorizer) Authorizer {
	return &authCache{
		authz: authz,
		// In practice, this cache should never come close to filling since the
		// authorization calls are kept for a minute at most.
		cache: tlru.New[[32]byte](tlru.ConstantCost[error], 64*1024),
	}
}

func (c *authCache) Authorize(ctx context.Context, subject Subject, action Action, object Object) error {
	authorizeCacheKey := hashAuthorizeCall(subject, action, object)

	var err error
	err, _, ok := c.cache.Get(authorizeCacheKey)
	if !ok {
		err = c.authz.Authorize(ctx, subject, action, object)
		// If there is a transient error such as a context cancellation, do not
		// cache it.
		if !errors.Is(err, context.Canceled) {
			// In case there is a caching bug, bound the TTL to 1 minute.
			c.cache.Set(authorizeCacheKey, err, time.Minute)
		}
	}

	return err
}

// Prepare returns the underlying PreparedAuthorized. The cache does not apply
// to prepared authorizations. These should be using a SQL filter, and
// therefore the cache is not needed.
func (c *authCache) Prepare(ctx context.Context, subject Subject, action Action, objectType string) (PreparedAuthorized, error) {
	return c.authz.Prepare(ctx, subject, action, objectType)
}

// rbacTraceAttributes are the attributes that are added to all spans created by
// the rbac package. These attributes should help to debug slow spans.
func rbacTraceAttributes(actor Subject, action Action, objectType string, extra ...attribute.KeyValue) trace.SpanStartOption {
	return trace.WithAttributes(
		append(extra,
			attribute.StringSlice("subject_roles", actor.SafeRoleNames()),
			attribute.Int("num_subject_roles", len(actor.SafeRoleNames())),
			attribute.Int("num_groups", len(actor.Groups)),
			attribute.String("scope", actor.SafeScopeName()),
			attribute.String("action", string(action)),
			attribute.String("object_type", objectType),
		)...)
}
