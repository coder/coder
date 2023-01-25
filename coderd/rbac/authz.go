package rbac

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/rego"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac/regosql"
	"github.com/coder/coder/coderd/tracing"
)

// ExpandableRoles is any type that can be expanded into a []Role. This is implemented
// as an interface so we can have RoleNames for user defined roles, and implement
// custom ExpandableRoles for system type users (eg autostart/autostop system role).
// We want a clear divide between the two types of roles so users have no codepath
// to interact or assign system roles.
//
// Note: We may also want to do the same thing with scopes to allow custom scope
// support unavailable to the user. Eg: Scope to a single resource.
type ExpandableRoles interface {
	Expand() ([]Role, error)
	// Names is for logging and tracing purposes, we want to know the human
	// names of the expanded roles.
	Names() []string
}

type Authorizer interface {
	ByRoleName(ctx context.Context, subjectID string, roleNames ExpandableRoles, scope ScopeName, groups []string, action Action, object Object) error
	PrepareByRoleName(ctx context.Context, subjectID string, roleNames ExpandableRoles, scope ScopeName, groups []string, action Action, objectType string) (PreparedAuthorized, error)
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
func Filter[O Objecter](ctx context.Context, auth Authorizer, subjID string, subjRoles ExpandableRoles, scope ScopeName, groups []string, action Action, objects []O) ([]O, error) {
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
		rbacTraceAttributes(subjRoles.Names(), len(groups), scope, action, objectType,
			// For filtering, we are only measuring the total time for the entire
			// set of objects. This and the 'PrepareByRoleName' span time
			// is all that is required to measure the performance of this
			// function on a per-object basis.
			attribute.Int("num_objects", len(objects)),
		),
	)
	defer span.End()

	// Running benchmarks on this function, it is **always** faster to call
	// auth.ByRoleName on <10 objects. This is because the overhead of
	// 'PrepareByRoleName'. Once we cross 10 objects, then it starts to become
	// faster
	if len(objects) < 10 {
		for _, o := range objects {
			rbacObj := o.RBACObject()
			if rbacObj.Type != objectType {
				return nil, xerrors.Errorf("object types must be uniform across the set (%s), found %s", objectType, rbacObj)
			}
			err := auth.ByRoleName(ctx, subjID, subjRoles, scope, groups, action, o.RBACObject())
			if err == nil {
				filtered = append(filtered, o)
			}
		}
		return filtered, nil
	}

	prepared, err := auth.PrepareByRoleName(ctx, subjID, subjRoles, scope, groups, action, objectType)
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
		}
	}

	return filtered, nil
}

// RegoAuthorizer will use a prepared rego query for performing authorize()
type RegoAuthorizer struct {
	query rego.PreparedEvalQuery

	authorizeHist *prometheus.HistogramVec
	prepareHist   prometheus.Histogram
}

var _ Authorizer = (*RegoAuthorizer)(nil)

var (
	// Load the policy from policy.rego in this directory.
	//
	//go:embed policy.rego
	policy    string
	queryOnce sync.Once
	query     rego.PreparedEvalQuery
)

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
		query: query,

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

// ByRoleName will expand all roleNames into roles before calling Authorize().
// This is the function intended to be used outside this package.
// The role is fetched from the builtin map located in memory.
func (a RegoAuthorizer) ByRoleName(ctx context.Context, subjectID string, roleNames ExpandableRoles, scope ScopeName, groups []string, action Action, object Object) error {
	start := time.Now()
	ctx, span := tracing.StartSpan(ctx,
		trace.WithTimestamp(start), // Reuse the time.Now for metric and trace
		rbacTraceAttributes(roleNames.Names(), len(groups), scope, action, object.Type,
			// For authorizing a single object, this data is useful to know how
			// complex our objects are getting.
			attribute.Int("object_num_groups", len(object.ACLGroupList)),
			attribute.Int("object_num_users", len(object.ACLUserList)),
		),
	)
	defer span.End()

	roles, err := roleNames.Expand()
	if err != nil {
		return err
	}

	scopeRole, err := ExpandScope(scope)
	if err != nil {
		return err
	}

	err = a.Authorize(ctx, subjectID, roles, scopeRole, groups, action, object)
	span.AddEvent("authorized", trace.WithAttributes(attribute.Bool("authorized", err == nil)))
	dur := time.Since(start)
	if err != nil {
		a.authorizeHist.WithLabelValues("false").Observe(dur.Seconds())
		return err
	}

	a.authorizeHist.WithLabelValues("true").Observe(dur.Seconds())
	return nil
}

// Authorize allows passing in custom Roles.
// This is really helpful for unit testing, as we can create custom roles to exercise edge cases.
func (a RegoAuthorizer) Authorize(ctx context.Context, subjectID string, roles []Role, scope Scope, groups []string, action Action, object Object) error {
	input := map[string]interface{}{
		"subject": authSubject{
			ID:     subjectID,
			Roles:  roles,
			Groups: groups,
			Scope:  scope,
		},
		"object": object,
		"action": action,
	}

	results, err := a.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w", err), input, results)
	}

	if !results.Allowed() {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
	}
	return nil
}

func (a RegoAuthorizer) PrepareByRoleName(ctx context.Context, subjectID string, roleNames ExpandableRoles, scope ScopeName, groups []string, action Action, objectType string) (PreparedAuthorized, error) {
	start := time.Now()
	ctx, span := tracing.StartSpan(ctx,
		trace.WithTimestamp(start),
		rbacTraceAttributes(roleNames.Names(), len(groups), scope, action, objectType),
	)
	defer span.End()

	roles, err := roleNames.Expand()
	if err != nil {
		return nil, err
	}

	scopeRole, err := ExpandScope(scope)
	if err != nil {
		return nil, err
	}

	prepared, err := a.Prepare(ctx, subjectID, roles, scopeRole, groups, action, objectType)
	if err != nil {
		return nil, err
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

// Prepare will partially execute the rego policy leaving the object fields unknown (except for the type).
// This will vastly speed up performance if batch authorization on the same type of objects is needed.
func (RegoAuthorizer) Prepare(ctx context.Context, subjectID string, roles []Role, scope Scope, groups []string, action Action, objectType string) (*PartialAuthorizer, error) {
	auth, err := newPartialAuthorizer(ctx, subjectID, roles, scope, groups, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("new partial authorizer: %w", err)
	}

	return auth, nil
}
