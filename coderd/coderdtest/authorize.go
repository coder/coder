package coderdtest

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

// RBACAsserter is a helper for asserting that the correct RBAC checks are
// performed. This struct is tied to a given user, and only authorizes calls
// for this user are checked.
type RBACAsserter struct {
	Subject rbac.Subject

	Recorder *RecordingAuthorizer
}

// AssertRBAC returns an RBACAsserter for the given user. This asserter will
// allow asserting that the correct RBAC checks are performed for the given user.
// All checks that are not run against this user will be ignored.
func AssertRBAC(t *testing.T, api *coderd.API, client *codersdk.Client) RBACAsserter {
	if client.SessionToken() == "" {
		t.Fatal("client must be logged in")
	}
	recorder, ok := api.Authorizer.(*RecordingAuthorizer)
	if !ok {
		t.Fatal("expected RecordingAuthorizer")
	}

	// We use the database directly to not cause additional auth checks on behalf
	// of the user. This does add authz checks on behalf of the system user, but
	// it is hard to avoid that.
	// nolint:gocritic
	ctx := dbauthz.AsSystemRestricted(context.Background())
	token := client.SessionToken()
	parts := strings.Split(token, "-")
	key, err := api.Database.GetAPIKeyByID(ctx, parts[0])
	require.NoError(t, err, "fetch client api key")

	roles, err := api.Database.GetAuthorizationUserRoles(ctx, key.UserID)
	require.NoError(t, err, "fetch user roles")

	return RBACAsserter{
		Subject: rbac.Subject{
			ID:     key.UserID.String(),
			Roles:  rbac.RoleNames(roles.Roles),
			Groups: roles.Groups,
			Scope:  rbac.ScopeName(key.Scope),
		},
		Recorder: recorder,
	}
}

// AllCalls is for debugging. If you are not sure where calls are coming from,
// call this and use a debugger or print them. They have small callstacks
// on them to help locate the 'Authorize' call.
// Only calls to Authorize by the given subject will be returned.
// Note that duplicate rbac calls are handled by the rbac.Cacher(), but
// will be recorded twice. So AllCalls() returns calls regardless if they
// were returned from the cached or not.
func (a RBACAsserter) AllCalls() []AuthCall {
	return a.Recorder.AllCalls(&a.Subject)
}

// AssertChecked will assert a given rbac check was performed. It does not care
// about order of checks, or any other checks. This is useful when you do not
// care about asserting every check that was performed.
func (a RBACAsserter) AssertChecked(t *testing.T, action rbac.Action, objects ...interface{}) {
	converted := a.convertObjects(t, objects...)
	pairs := make([]ActionObjectPair, 0, len(converted))
	for _, obj := range converted {
		pairs = append(pairs, a.Recorder.Pair(action, obj))
	}
	a.Recorder.AssertOutOfOrder(t, a.Subject, pairs...)
}

// AssertInOrder must be called in the correct order of authz checks. If the objects
// or actions are not in the correct order, the test will fail.
func (a RBACAsserter) AssertInOrder(t *testing.T, action rbac.Action, objects ...interface{}) {
	converted := a.convertObjects(t, objects...)
	pairs := make([]ActionObjectPair, 0, len(converted))
	for _, obj := range converted {
		pairs = append(pairs, a.Recorder.Pair(action, obj))
	}
	a.Recorder.AssertActor(t, a.Subject, pairs...)
}

// convertObjects converts the codersdk types to rbac.Object. Unfortunately
// does not have type safety, and instead uses a t.Fatal to enforce types.
func (RBACAsserter) convertObjects(t *testing.T, objs ...interface{}) []rbac.Object {
	converted := make([]rbac.Object, 0, len(objs))
	for _, obj := range objs {
		var robj rbac.Object
		switch obj := obj.(type) {
		case rbac.Object:
			robj = obj
		case rbac.Objecter:
			robj = obj.RBACObject()
		case codersdk.TemplateVersion:
			robj = rbac.ResourceTemplate.InOrg(obj.OrganizationID)
		case codersdk.User:
			robj = rbac.ResourceUserObject(obj.ID)
		case codersdk.Workspace:
			robj = rbac.ResourceWorkspace.WithID(obj.ID).InOrg(obj.OrganizationID).WithOwner(obj.OwnerID.String())
		default:
			t.Fatalf("unsupported type %T to convert to rbac.Object, add the implementation", obj)
		}
		converted = append(converted, robj)
	}
	return converted
}

// Reset will clear all previously recorded authz calls.
// This is helpful when wanting to ignore checks run in test setup.
func (a RBACAsserter) Reset() RBACAsserter {
	a.Recorder.Reset()
	return a
}

type AuthCall struct {
	rbac.AuthCall

	asserted bool
	// callers is a small stack trace for debugging.
	callers []string
}

var _ rbac.Authorizer = (*RecordingAuthorizer)(nil)

// RecordingAuthorizer wraps any rbac.Authorizer and records all Authorize()
// calls made. This is useful for testing as these calls can later be asserted.
type RecordingAuthorizer struct {
	sync.RWMutex
	Called  []AuthCall
	Wrapped rbac.Authorizer
}

type ActionObjectPair struct {
	Action rbac.Action
	Object rbac.Object
}

// Pair is on the RecordingAuthorizer to be easy to find and keep the pkg
// interface smaller.
func (*RecordingAuthorizer) Pair(action rbac.Action, object rbac.Objecter) ActionObjectPair {
	return ActionObjectPair{
		Action: action,
		Object: object.RBACObject(),
	}
}

// AllAsserted returns an error if all calls to Authorize() have not been
// asserted and checked. This is useful for testing to ensure that all
// Authorize() calls are checked in the unit test.
func (r *RecordingAuthorizer) AllAsserted() error {
	r.RLock()
	defer r.RUnlock()
	missed := []AuthCall{}
	for _, c := range r.Called {
		if !c.asserted {
			missed = append(missed, c)
		}
	}

	if len(missed) > 0 {
		return xerrors.Errorf("missed calls: %+v", missed)
	}
	return nil
}

// AllCalls is useful for debugging.
func (r *RecordingAuthorizer) AllCalls(actor *rbac.Subject) []AuthCall {
	r.RLock()
	defer r.RUnlock()

	called := make([]AuthCall, 0, len(r.Called))
	for _, c := range r.Called {
		if actor != nil && !c.Actor.Equal(*actor) {
			continue
		}
		called = append(called, c)
	}
	return called
}

// AssertOutOfOrder asserts that the given actor performed the given action
// on the given objects. It does not care about the order of the calls.
// When marking authz calls as asserted, it will mark the first matching
// calls first.
func (r *RecordingAuthorizer) AssertOutOfOrder(t *testing.T, actor rbac.Subject, did ...ActionObjectPair) {
	r.Lock()
	defer r.Unlock()

	for _, do := range did {
		found := false
		// Find the first non-asserted call that matches the actor, action, and object.
		for i, call := range r.Called {
			if !call.asserted && call.Actor.Equal(actor) && call.Action == do.Action && call.Object.Equal(do.Object) {
				r.Called[i].asserted = true
				found = true
				break
			}
		}
		require.True(t, found, "assertion missing: %s %s %s", actor, do.Action, do.Object)
	}
}

// AssertActor asserts in order. If the order of authz calls does not match,
// this will fail.
func (r *RecordingAuthorizer) AssertActor(t *testing.T, actor rbac.Subject, did ...ActionObjectPair) {
	r.Lock()
	defer r.Unlock()
	ptr := 0
	for i, call := range r.Called {
		if ptr == len(did) {
			// Finished all assertions
			return
		}
		if call.Actor.ID == actor.ID {
			action, object := did[ptr].Action, did[ptr].Object
			assert.Equalf(t, action, call.Action, "assert action %d", ptr)
			assert.Equalf(t, object, call.Object, "assert object %d", ptr)
			r.Called[i].asserted = true
			ptr++
		}
	}

	assert.Equalf(t, len(did), ptr, "assert actor: didn't find all actions, %d missing actions", len(did)-ptr)
}

// recordAuthorize is the internal method that records the Authorize() call.
func (r *RecordingAuthorizer) recordAuthorize(subject rbac.Subject, action rbac.Action, object rbac.Object) {
	r.Lock()
	defer r.Unlock()

	r.Called = append(r.Called, AuthCall{
		AuthCall: rbac.AuthCall{
			Actor:  subject,
			Action: action,
			Object: object,
		},
		callers: []string{
			// This is a decent stack trace for debugging.
			// Some dbauthz calls are a bit nested, so we skip a few.
			caller(2),
			caller(3),
			caller(4),
			caller(5),
		},
	})
}

func caller(skip int) string {
	pc, file, line, ok := runtime.Caller(skip + 1)
	i := strings.Index(file, "coder")
	if i >= 0 {
		file = file[i:]
	}
	str := fmt.Sprintf("%s:%d", file, line)
	if ok {
		f := runtime.FuncForPC(pc)
		str += " | " + filepath.Base(f.Name())
	}
	return str
}

func (r *RecordingAuthorizer) Authorize(ctx context.Context, subject rbac.Subject, action rbac.Action, object rbac.Object) error {
	r.recordAuthorize(subject, action, object)
	if r.Wrapped == nil {
		panic("Developer error: RecordingAuthorizer.Wrapped is nil")
	}
	return r.Wrapped.Authorize(ctx, subject, action, object)
}

func (r *RecordingAuthorizer) Prepare(ctx context.Context, subject rbac.Subject, action rbac.Action, objectType string) (rbac.PreparedAuthorized, error) {
	r.RLock()
	defer r.RUnlock()
	if r.Wrapped == nil {
		panic("Developer error: RecordingAuthorizer.Wrapped is nil")
	}

	prep, err := r.Wrapped.Prepare(ctx, subject, action, objectType)
	if err != nil {
		return nil, err
	}
	return &PreparedRecorder{
		rec:     r,
		prepped: prep,
		subject: subject,
		action:  action,
	}, nil
}

// Reset clears the recorded Authorize() calls.
func (r *RecordingAuthorizer) Reset() {
	r.Lock()
	defer r.Unlock()
	r.Called = nil
}

// PreparedRecorder is the prepared version of the RecordingAuthorizer.
// It records the Authorize() calls to the original recorder. If the caller
// uses CompileToSQL, all recording stops. This is to support parity between
// memory and SQL backed dbs.
type PreparedRecorder struct {
	rec     *RecordingAuthorizer
	prepped rbac.PreparedAuthorized
	subject rbac.Subject
	action  rbac.Action

	rw       sync.Mutex
	usingSQL bool
}

func (s *PreparedRecorder) Authorize(ctx context.Context, object rbac.Object) error {
	s.rw.Lock()
	defer s.rw.Unlock()

	if !s.usingSQL {
		s.rec.recordAuthorize(s.subject, s.action, object)
	}
	return s.prepped.Authorize(ctx, object)
}

func (s *PreparedRecorder) CompileToSQL(ctx context.Context, cfg regosql.ConvertConfig) (string, error) {
	s.rw.Lock()
	defer s.rw.Unlock()

	s.usingSQL = true
	return s.prepped.CompileToSQL(ctx, cfg)
}

// FakeAuthorizer is an Authorizer that always returns the same error.
type FakeAuthorizer struct {
	// AlwaysReturn is the error that will be returned by Authorize.
	AlwaysReturn error
}

var _ rbac.Authorizer = (*FakeAuthorizer)(nil)

func (d *FakeAuthorizer) Authorize(_ context.Context, _ rbac.Subject, _ rbac.Action, _ rbac.Object) error {
	return d.AlwaysReturn
}

func (d *FakeAuthorizer) Prepare(_ context.Context, subject rbac.Subject, action rbac.Action, _ string) (rbac.PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original: d,
		Subject:  subject,
		Action:   action,
	}, nil
}

var _ rbac.PreparedAuthorized = (*fakePreparedAuthorizer)(nil)

// fakePreparedAuthorizer is the prepared version of a FakeAuthorizer. It will
// return the same error as the original FakeAuthorizer.
type fakePreparedAuthorizer struct {
	sync.RWMutex
	Original *FakeAuthorizer
	Subject  rbac.Subject
	Action   rbac.Action
}

func (f *fakePreparedAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return f.Original.Authorize(ctx, f.Subject, f.Action, object)
}

// CompileToSQL returns a compiled version of the authorizer that will work for
// in memory databases. This fake version will not work against a SQL database.
func (*fakePreparedAuthorizer) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "not a valid sql string", nil
}

// Random rbac helper funcs

func RandomRBACAction() rbac.Action {
	all := rbac.AllActions()
	return all[must(cryptorand.Intn(len(all)))]
}

func RandomRBACObject() rbac.Object {
	return rbac.Object{
		ID:    uuid.NewString(),
		Owner: uuid.NewString(),
		OrgID: uuid.NewString(),
		Type:  randomRBACType(),
		ACLUserList: map[string][]rbac.Action{
			namesgenerator.GetRandomName(1): {RandomRBACAction()},
		},
		ACLGroupList: map[string][]rbac.Action{
			namesgenerator.GetRandomName(1): {RandomRBACAction()},
		},
	}
}

func randomRBACType() string {
	all := []string{
		rbac.ResourceWorkspace.Type,
		rbac.ResourceWorkspaceExecution.Type,
		rbac.ResourceWorkspaceApplicationConnect.Type,
		rbac.ResourceAuditLog.Type,
		rbac.ResourceTemplate.Type,
		rbac.ResourceGroup.Type,
		rbac.ResourceFile.Type,
		rbac.ResourceProvisionerDaemon.Type,
		rbac.ResourceOrganization.Type,
		rbac.ResourceRoleAssignment.Type,
		rbac.ResourceOrgRoleAssignment.Type,
		rbac.ResourceAPIKey.Type,
		rbac.ResourceUser.Type,
		rbac.ResourceUserData.Type,
		rbac.ResourceOrganizationMember.Type,
		rbac.ResourceWildcard.Type,
		rbac.ResourceLicense.Type,
		rbac.ResourceDeploymentValues.Type,
		rbac.ResourceReplicas.Type,
		rbac.ResourceDebugInfo.Type,
	}
	return all[must(cryptorand.Intn(len(all)))]
}

func RandomRBACSubject() rbac.Subject {
	return rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleMember()},
		Groups: []string{namesgenerator.GetRandomName(1)},
		Scope:  rbac.ScopeAll,
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type FakeAccessControlStore struct{}

func (FakeAccessControlStore) GetTemplateAccessControl(t database.Template) dbauthz.TemplateAccessControl {
	return dbauthz.TemplateAccessControl{
		RequireActiveVersion: t.RequireActiveVersion,
	}
}

func (FakeAccessControlStore) SetTemplateAccessControl(context.Context, database.Store, uuid.UUID, dbauthz.TemplateAccessControl) error {
	panic("not implemented")
}

func AccessControlStorePointer() *atomic.Pointer[dbauthz.AccessControlStore] {
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var tacs dbauthz.AccessControlStore = FakeAccessControlStore{}
	acs.Store(&tacs)
	return acs
}
