package agentapi_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// unavailableMessage is the single sentinel every tuple, access, and state
// failure must answer with, so a parent cannot tell an unknown declaration from
// one it does not own, a superseded generation, or a fenced launcher.
const unavailableMessage = "subagent execution is unavailable"

func TestSubagentExecutionAPIValidation(t *testing.T) {
	t.Parallel()

	// The mock store has no expectations, so any of these requests reaching the
	// database at all fails the test. Rejection must happen before the elevated
	// database call.
	newAPI := func(t *testing.T, agent database.WorkspaceAgent) *agentapi.SubagentExecutionAPI {
		t.Helper()

		return &agentapi.SubagentExecutionAPI{
			AgentFn:  func(context.Context) (database.WorkspaceAgent, error) { return agent, nil },
			Log:      testutil.Logger(t),
			Clock:    quartz.NewMock(t),
			Database: dbmock.NewMockStore(gomock.NewController(t)),
		}
	}

	parentAgent := database.WorkspaceAgent{ID: uuid.New()}
	childAgent := database.WorkspaceAgent{
		ID:       uuid.New(),
		ParentID: uuid.NullUUID{UUID: parentAgent.ID, Valid: true},
	}

	executionID := uuid.New()
	generation := uuid.New()

	validAcquire := func() *proto.AcquireSubagentExecutionRequest {
		return &proto.AcquireSubagentExecutionRequest{
			ExecutionId: executionID[:],
			Generation:  generation[:],
		}
	}
	validReport := func() *proto.ReportSubagentExecutionStatusRequest {
		return &proto.ReportSubagentExecutionStatusRequest{
			ExecutionId:        executionID[:],
			Generation:         generation[:],
			AcquisitionVersion: 1,
			Status:             proto.ReportSubagentExecutionStatusRequest_RUNNING,
		}
	}

	t.Run("Acquire", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			agent   database.WorkspaceAgent
			req     *proto.AcquireSubagentExecutionRequest
			message string
		}{
			{
				name:    "NilRequest",
				agent:   parentAgent,
				req:     nil,
				message: unavailableMessage,
			},
			{
				name:  "MalformedExecutionID",
				agent: parentAgent,
				req: &proto.AcquireSubagentExecutionRequest{
					ExecutionId: []byte("not-a-uuid"),
					Generation:  generation[:],
				},
				message: unavailableMessage,
			},
			{
				name:  "EmptyExecutionID",
				agent: parentAgent,
				req: &proto.AcquireSubagentExecutionRequest{
					Generation: generation[:],
				},
				message: unavailableMessage,
			},
			{
				name:  "MalformedGeneration",
				agent: parentAgent,
				req: &proto.AcquireSubagentExecutionRequest{
					ExecutionId: executionID[:],
					Generation:  []byte{0x01, 0x02},
				},
				message: unavailableMessage,
			},
			{
				name:    "ChildCaller",
				agent:   childAgent,
				req:     validAcquire(),
				message: "child agents cannot control subagent executions",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				resp, err := newAPI(t, tc.agent).AcquireSubagentExecution(ctx, tc.req)
				require.Error(t, err)
				require.Equal(t, tc.message, err.Error())
				require.Nil(t, resp)
			})
		}
	})

	t.Run("Report", func(t *testing.T) {
		t.Parallel()

		withReport := func(mutate func(req *proto.ReportSubagentExecutionStatusRequest)) *proto.ReportSubagentExecutionStatusRequest {
			req := validReport()
			mutate(req)
			return req
		}

		for _, tc := range []struct {
			name    string
			agent   database.WorkspaceAgent
			req     *proto.ReportSubagentExecutionStatusRequest
			message string
		}{
			{
				name:    "NilRequest",
				agent:   parentAgent,
				req:     nil,
				message: unavailableMessage,
			},
			{
				name:  "MalformedExecutionID",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.ExecutionId = []byte("not-a-uuid")
				}),
				message: unavailableMessage,
			},
			{
				name:  "MalformedGeneration",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Generation = []byte{0xff}
				}),
				message: unavailableMessage,
			},
			{
				name:    "ChildCaller",
				agent:   childAgent,
				req:     validReport(),
				message: "child agents cannot control subagent executions",
			},
			{
				name:  "ZeroAcquisitionVersion",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.AcquisitionVersion = 0
				}),
				message: "subagent execution acquisition version is required",
			},
			{
				name:  "NegativeAcquisitionVersion",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.AcquisitionVersion = -1
				}),
				message: "subagent execution acquisition version is required",
			},
			{
				name:  "StatusUnspecified",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_STATUS_UNSPECIFIED
				}),
				message: "subagent execution status is not reportable",
			},
			{
				// 'starting' belongs to the acquisition: a launcher that wants it
				// again must acquire again, which bumps the fencing version.
				name:  "StatusStarting",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_STARTING
				}),
				message: "subagent execution status is not reportable",
			},
			{
				name:  "StatusUnknown",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_Status(99)
				}),
				message: "subagent execution status is not reportable",
			},
			{
				name:  "ErrorTooLongASCII",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_FAILED
					req.Error = strings.Repeat("a", 4097)
				}),
				message: "subagent execution error is too long",
			},
			{
				// 2049 two-byte runes are 4098 bytes: the limit is on octets, not
				// runes, and the boundary must not be crossed by a partial rune.
				name:  "ErrorTooLongTwoByteRunes",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_FAILED
					req.Error = strings.Repeat("é", 2049)
				}),
				message: "subagent execution error is too long",
			},
			{
				// 1025 four-byte runes are 4100 bytes.
				name:  "ErrorTooLongFourByteRunes",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_FAILED
					req.Error = strings.Repeat("😀", 1025)
				}),
				message: "subagent execution error is too long",
			},
			{
				name:  "ErrorInvalidUTF8",
				agent: parentAgent,
				req: withReport(func(req *proto.ReportSubagentExecutionStatusRequest) {
					req.Status = proto.ReportSubagentExecutionStatusRequest_FAILED
					req.Error = string([]byte{0xff, 0xfe})
				}),
				message: unavailableMessage,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				resp, err := newAPI(t, tc.agent).ReportSubagentExecutionStatus(ctx, tc.req)
				require.Error(t, err)
				require.Equal(t, tc.message, err.Error())
				require.Nil(t, resp)
			})
		}
	})
}

// TestSubagentExecutionAPIParameters pins exactly what the handlers hand to the
// database: the authenticated parent's ID, the requested tuple, and the clock's
// timestamp.
func TestSubagentExecutionAPIParameters(t *testing.T) {
	t.Parallel()

	newAPI := func(t *testing.T, agent database.WorkspaceAgent) (*agentapi.SubagentExecutionAPI, *dbmock.MockStore, time.Time) {
		t.Helper()

		clock := quartz.NewMock(t)
		clock.Set(dbtime.Time(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)))
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		return &agentapi.SubagentExecutionAPI{
			AgentFn:  func(context.Context) (database.WorkspaceAgent, error) { return agent, nil },
			Log:      testutil.Logger(t),
			Clock:    clock,
			Database: mDB,
		}, mDB, dbtime.Time(clock.Now())
	}

	parentAgent := database.WorkspaceAgent{ID: uuid.New()}
	executionID := uuid.New()
	generation := uuid.New()

	t.Run("Acquire", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		api, mDB, now := newAPI(t, parentAgent)

		childID := uuid.New()
		authToken := uuid.New()
		mDB.EXPECT().AcquireWorkspaceAgentSubagentExecution(gomock.Any(), database.AcquireWorkspaceAgentSubagentExecutionParams{
			WorkspaceBuildID: generation,
			DeclarationID:    executionID,
			ParentAgentID:    parentAgent.ID,
			Now:              now,
		}).Return(database.AcquireWorkspaceAgentSubagentExecutionRow{
			ChildAgentID:       childID,
			AuthToken:          authToken,
			AcquisitionVersion: 3,
		}, nil)

		resp, err := api.AcquireSubagentExecution(ctx, &proto.AcquireSubagentExecutionRequest{
			ExecutionId: executionID[:],
			Generation:  generation[:],
		})
		require.NoError(t, err)
		require.Equal(t, childID[:], resp.ChildAgentId)
		require.Equal(t, authToken.String(), resp.AuthToken)
		require.Equal(t, int64(3), resp.AcquisitionVersion)
	})

	t.Run("Report", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name     string
			status   proto.ReportSubagentExecutionStatusRequest_Status
			expected database.SubagentExecutionStatus
		}{
			{
				name:     "Running",
				status:   proto.ReportSubagentExecutionStatusRequest_RUNNING,
				expected: database.SubagentExecutionStatusRunning,
			},
			{
				name:     "Stopping",
				status:   proto.ReportSubagentExecutionStatusRequest_STOPPING,
				expected: database.SubagentExecutionStatusStopping,
			},
			{
				name:     "Stopped",
				status:   proto.ReportSubagentExecutionStatusRequest_STOPPED,
				expected: database.SubagentExecutionStatusStopped,
			},
			{
				name:     "Failed",
				status:   proto.ReportSubagentExecutionStatusRequest_FAILED,
				expected: database.SubagentExecutionStatusFailed,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				api, mDB, now := newAPI(t, parentAgent)

				mDB.EXPECT().ReportWorkspaceAgentSubagentExecutionStatus(gomock.Any(), database.ReportWorkspaceAgentSubagentExecutionStatusParams{
					WorkspaceBuildID:   generation,
					DeclarationID:      executionID,
					ParentAgentID:      parentAgent.ID,
					AcquisitionVersion: 7,
					Status:             tc.expected,
					LastError:          "driver log tail",
					Now:                now,
				}).Return(database.ReportWorkspaceAgentSubagentExecutionStatusRow{}, nil)

				resp, err := api.ReportSubagentExecutionStatus(ctx, &proto.ReportSubagentExecutionStatusRequest{
					ExecutionId:        executionID[:],
					Generation:         generation[:],
					AcquisitionVersion: 7,
					Status:             tc.status,
					Error:              "driver log tail",
				})
				require.NoError(t, err)
				require.NotNil(t, resp)
			})
		}
	})

	// An error of exactly the column's octet limit is accepted, and it is passed
	// through untouched rather than truncated.
	t.Run("Report/ErrorAtLimit", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name  string
			value string
		}{
			{name: "ASCII", value: strings.Repeat("a", 4096)},
			{name: "TwoByteRunes", value: strings.Repeat("é", 2048)},
			{name: "FourByteRunes", value: strings.Repeat("😀", 1024)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Len(t, []byte(tc.value), 4096)

				ctx := testutil.Context(t, testutil.WaitShort)
				api, mDB, now := newAPI(t, parentAgent)

				mDB.EXPECT().ReportWorkspaceAgentSubagentExecutionStatus(gomock.Any(), database.ReportWorkspaceAgentSubagentExecutionStatusParams{
					WorkspaceBuildID:   generation,
					DeclarationID:      executionID,
					ParentAgentID:      parentAgent.ID,
					AcquisitionVersion: 1,
					Status:             database.SubagentExecutionStatusFailed,
					LastError:          tc.value,
					Now:                now,
				}).Return(database.ReportWorkspaceAgentSubagentExecutionStatusRow{}, nil)

				_, err := api.ReportSubagentExecutionStatus(ctx, &proto.ReportSubagentExecutionStatusRequest{
					ExecutionId:        executionID[:],
					Generation:         generation[:],
					AcquisitionVersion: 1,
					Status:             proto.ReportSubagentExecutionStatusRequest_FAILED,
					Error:              tc.value,
				})
				require.NoError(t, err)
			})
		}
	})
}

// executionFixture is a real declaration a parent agent can acquire, backed by
// the authorizing store and the agent's own actor, so the tests exercise the same
// authorization path a connected agent does.
type executionFixture struct {
	rawDB         database.Store
	ctx           context.Context
	clock         *quartz.Mock
	api           *agentapi.SubagentExecutionAPI
	workspace     database.WorkspaceTable
	build         database.WorkspaceBuild
	parent        database.WorkspaceAgent
	child         database.WorkspaceAgent
	declarationID uuid.UUID
	// newBuild publishes a further build for the same workspace, which makes the
	// fixture's generation stale.
	newBuild func(t *testing.T, buildNumber int32, transition database.WorkspaceTransition) database.WorkspaceBuild
}

func (f executionFixture) acquireRequest() *proto.AcquireSubagentExecutionRequest {
	return &proto.AcquireSubagentExecutionRequest{
		ExecutionId: f.declarationID[:],
		Generation:  f.build.ID[:],
	}
}

func (f executionFixture) reportRequest(version int64, status proto.ReportSubagentExecutionStatusRequest_Status) *proto.ReportSubagentExecutionStatusRequest {
	return &proto.ReportSubagentExecutionStatusRequest{
		ExecutionId:        f.declarationID[:],
		Generation:         f.build.ID[:],
		AcquisitionVersion: version,
		Status:             status,
	}
}

func newExecutionFixture(t *testing.T) executionFixture {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	newBuild := func(t *testing.T, buildNumber int32, transition database.WorkspaceTransition) database.WorkspaceBuild {
		t.Helper()

		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		return dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			BuildNumber:       buildNumber,
			JobID:             job.ID,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: version.ID,
			InitiatorID:       user.ID,
			Transition:        transition,
		})
	}
	build := newBuild(t, 1, database.WorkspaceTransitionStart)
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: build.JobID,
	})
	parent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
	child := dbgen.WorkspaceSubAgent(t, db, parent, database.WorkspaceAgent{
		Name:               "execution-child",
		ExecutionIsolation: true,
	})
	declarationID := uuid.New()
	_, err := db.InsertWorkspaceAgentSubagentExecution(ctx, database.InsertWorkspaceAgentSubagentExecutionParams{
		WorkspaceBuildID:      build.ID,
		DeclarationID:         declarationID,
		ParentAgentID:         parent.ID,
		ChildAgentID:          child.ID,
		Driver:                "docker",
		DriverProtocol:        1,
		SharedHostPath:        "/home/coder/project",
		SharedChildPath:       "/workspace/project",
		StartupTimeoutSeconds: 30,
		RestartPolicy:         "on-failure",
	})
	require.NoError(t, err)

	// The workspace owner's roles: 'organization-workspace-access' is what grants
	// update on a workspace the actor owns, which is what the acquisition and the
	// report authorize against.
	roles, err := rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgWorkspaceAccess(org.ID)}.Expand()
	require.NoError(t, err)
	// The same actor a connected agent gets from the API token middleware: the
	// workspace owner's roles narrowed to this workspace by the agent scope.
	agentSubject := rbac.Subject{
		ID:    user.ID.String(),
		Roles: rbac.Roles(roles),
		Scope: rbac.WorkspaceAgentScope(rbac.WorkspaceAgentScopeParams{
			WorkspaceID: workspace.ID,
			OwnerID:     user.ID,
			TemplateID:  template.ID,
			VersionID:   version.ID,
		}),
	}.WithCachedASTValue()

	accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControlStore.Store(&acs)
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, accessControlStore)

	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now())

	return executionFixture{
		rawDB:         db,
		ctx:           dbauthz.As(ctx, agentSubject),
		clock:         clock,
		workspace:     workspace,
		build:         build,
		parent:        parent,
		child:         child,
		declarationID: declarationID,
		newBuild:      newBuild,
		api: &agentapi.SubagentExecutionAPI{
			AgentFn:  func(context.Context) (database.WorkspaceAgent, error) { return parent, nil },
			Log:      logger,
			Clock:    clock,
			Database: authzDB,
		},
	}
}

func TestSubagentExecutionAPIAcquire(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		fixture := newExecutionFixture(t)

		resp, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
		require.NoError(t, err)
		require.Equal(t, fixture.child.ID[:], resp.ChildAgentId)
		require.Equal(t, fixture.child.AuthToken.String(), resp.AuthToken)
		require.Equal(t, int64(1), resp.AcquisitionVersion)
		// The child's token is its own. Handing back the parent's token would give
		// the launcher the parent's authority.
		require.NotEqual(t, fixture.parent.AuthToken.String(), resp.AuthToken)

		status, err := fixture.rawDB.GetWorkspaceAgentSubagentExecutionStatus(fixture.ctx, database.GetWorkspaceAgentSubagentExecutionStatusParams{
			WorkspaceBuildID: fixture.build.ID,
			DeclarationID:    fixture.declarationID,
			ParentAgentID:    fixture.parent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, string(database.SubagentExecutionStatusStarting), status.Status)
		require.Equal(t, int64(1), status.AcquisitionVersion)
		require.True(t, status.LastAcquiredAt.Valid)
		require.Equal(t, fixture.clock.Now().UTC(), status.LastAcquiredAt.Time.UTC())

		// Re-acquiring fences the previous launcher.
		second, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
		require.NoError(t, err)
		require.Equal(t, int64(2), second.AcquisitionVersion)
	})

	// Every one of these is answered with the same sentinel, so a parent cannot
	// use the error to learn which part of the tuple it got wrong.
	t.Run("Unavailable", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			request func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest
		}{
			{
				name: "UnknownDeclaration",
				request: func(_ *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					other := uuid.New()
					req := fixture.acquireRequest()
					req.ExecutionId = other[:]
					return req
				},
			},
			{
				name: "UnknownGeneration",
				request: func(_ *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					other := uuid.New()
					req := fixture.acquireRequest()
					req.Generation = other[:]
					return req
				},
			},
			{
				name: "StaleGeneration",
				request: func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					fixture.newBuild(t, 2, database.WorkspaceTransitionStart)
					return fixture.acquireRequest()
				},
			},
			{
				name: "StoppedGeneration",
				request: func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					fixture.newBuild(t, 2, database.WorkspaceTransitionStop)
					return fixture.acquireRequest()
				},
			},
			{
				name: "WrongParent",
				request: func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					// Another agent on the same build, so only the parent differs.
					other := dbgen.WorkspaceAgent(t, fixture.rawDB, database.WorkspaceAgent{
						ResourceID: fixture.parentResourceID(t),
					})
					fixture.api.AgentFn = func(context.Context) (database.WorkspaceAgent, error) { return other, nil }
					return fixture.acquireRequest()
				},
			},
			{
				name: "DeletedChild",
				request: func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					require.NoError(t, fixture.rawDB.DeleteWorkspaceSubAgentByID(fixture.ctx, fixture.child.ID))
					return fixture.acquireRequest()
				},
			},
			{
				name: "StoppingExecution",
				request: func(t *testing.T, fixture executionFixture) *proto.AcquireSubagentExecutionRequest {
					// A shutting-down execution is never restarted by an
					// acquisition.
					acquired, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
					require.NoError(t, err)
					_, err = fixture.api.ReportSubagentExecutionStatus(fixture.ctx,
						fixture.reportRequest(acquired.AcquisitionVersion, proto.ReportSubagentExecutionStatusRequest_STOPPING))
					require.NoError(t, err)
					return fixture.acquireRequest()
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fixture := newExecutionFixture(t)
				req := tc.request(t, fixture)

				resp, err := fixture.api.AcquireSubagentExecution(fixture.ctx, req)
				require.Error(t, err)
				require.Equal(t, unavailableMessage, err.Error())
				require.Nil(t, resp)
			})
		}
	})
}

// parentResourceID returns the resource the fixture's parent agent lives on.
func (f executionFixture) parentResourceID(t *testing.T) uuid.UUID {
	t.Helper()

	agent, err := f.rawDB.GetWorkspaceAgentByID(f.ctx, f.parent.ID)
	require.NoError(t, err)
	return agent.ResourceID
}

func TestSubagentExecutionAPIReport(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		fixture := newExecutionFixture(t)
		acquired, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
		require.NoError(t, err)

		fixture.clock.Advance(time.Minute)
		req := fixture.reportRequest(acquired.AcquisitionVersion, proto.ReportSubagentExecutionStatusRequest_RUNNING)
		resp, err := fixture.api.ReportSubagentExecutionStatus(fixture.ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		status, err := fixture.rawDB.GetWorkspaceAgentSubagentExecutionStatus(fixture.ctx, database.GetWorkspaceAgentSubagentExecutionStatusParams{
			WorkspaceBuildID: fixture.build.ID,
			DeclarationID:    fixture.declarationID,
			ParentAgentID:    fixture.parent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, string(database.SubagentExecutionStatusRunning), status.Status)
		require.Equal(t, acquired.AcquisitionVersion, status.AcquisitionVersion)
		require.True(t, status.LastReportedAt.Valid)
		require.Equal(t, fixture.clock.Now().UTC(), status.LastReportedAt.Time.UTC())

		// The failure string is stored as reported, without truncation.
		failure := strings.Repeat("b", 4096)
		failed := fixture.reportRequest(acquired.AcquisitionVersion, proto.ReportSubagentExecutionStatusRequest_FAILED)
		failed.Error = failure
		_, err = fixture.api.ReportSubagentExecutionStatus(fixture.ctx, failed)
		require.NoError(t, err)

		status, err = fixture.rawDB.GetWorkspaceAgentSubagentExecutionStatus(fixture.ctx, database.GetWorkspaceAgentSubagentExecutionStatusParams{
			WorkspaceBuildID: fixture.build.ID,
			DeclarationID:    fixture.declarationID,
			ParentAgentID:    fixture.parent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, string(database.SubagentExecutionStatusFailed), status.Status)
		require.Equal(t, failure, status.LastError)
	})

	t.Run("Unavailable", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			request func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest
		}{
			{
				name: "UnknownDeclaration",
				request: func(_ *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					other := uuid.New()
					req := fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
					req.ExecutionId = other[:]
					return req
				},
			},
			{
				name: "StaleAcquisitionVersion",
				request: func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					// A second acquisition fences this launcher.
					_, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
					require.NoError(t, err)
					return fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
			{
				name: "FutureAcquisitionVersion",
				request: func(_ *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					return fixture.reportRequest(version+1, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
			{
				name: "RefusedTransition",
				request: func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					// 'stopped' is terminal for a launcher: only a new
					// acquisition may run again. It is reached through
					// 'stopping', which is the only successor of an acquisition's
					// 'starting' that leads there.
					_, err := fixture.api.ReportSubagentExecutionStatus(fixture.ctx,
						fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_STOPPING))
					require.NoError(t, err)
					_, err = fixture.api.ReportSubagentExecutionStatus(fixture.ctx,
						fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_STOPPED))
					require.NoError(t, err)
					return fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
			{
				name: "WrongParent",
				request: func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					other := dbgen.WorkspaceAgent(t, fixture.rawDB, database.WorkspaceAgent{
						ResourceID: fixture.parentResourceID(t),
					})
					fixture.api.AgentFn = func(context.Context) (database.WorkspaceAgent, error) { return other, nil }
					return fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
			{
				name: "StaleGenerationRunning",
				request: func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					// A launcher on a superseded generation may record that it is
					// going away, but never that the child is up.
					fixture.newBuild(t, 2, database.WorkspaceTransitionStart)
					return fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
			{
				name: "DeletedChild",
				request: func(t *testing.T, fixture executionFixture, version int64) *proto.ReportSubagentExecutionStatusRequest {
					require.NoError(t, fixture.rawDB.DeleteWorkspaceSubAgentByID(fixture.ctx, fixture.child.ID))
					return fixture.reportRequest(version, proto.ReportSubagentExecutionStatusRequest_RUNNING)
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fixture := newExecutionFixture(t)
				acquired, err := fixture.api.AcquireSubagentExecution(fixture.ctx, fixture.acquireRequest())
				require.NoError(t, err)

				req := tc.request(t, fixture, acquired.AcquisitionVersion)
				resp, err := fixture.api.ReportSubagentExecutionStatus(fixture.ctx, req)
				require.Error(t, err)
				require.Equal(t, unavailableMessage, err.Error())
				require.Nil(t, resp)
			})
		}
	})
}
