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
	"github.com/coder/coder/coderd/util/slice"
)

// Subject is a struct that contains all the elements of a subject in an rbac
// authorize.
type Subject struct {
	ID     string
	Roles  ExpandableRoles
	Groups []string
	Scope  ExpandableScope
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
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w", err), subject, action, object, results)
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
