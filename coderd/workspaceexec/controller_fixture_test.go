package workspaceexec_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type controllerFixture struct {
	api         *coderd.API
	provisioner io.Closer
	db          database.Store
	sqlDB       *sql.DB
	client      *codersdk.Client
	workspace   codersdk.Workspace
	agentID     uuid.UUID
	agent       *controllerHTTPAgent
	clock       *quartz.Mock
	controller  *workspaceexec.Controller
}

type controllerHTTPAgent struct {
	server       *httptest.Server
	disconnected atomic.Bool
	beforeBundle func(context.Context) error
}

func (a *controllerHTTPAgent) request(ctx context.Context, method, path string, input, output any) error {
	if a.disconnected.Load() {
		return xerrors.New("agent is unreachable")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, a.server.URL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	res, err := a.server.Client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	if raw, ok := output.(*[]byte); ok {
		*raw, err = io.ReadAll(res.Body)
		return err
	}
	//nolint:gocritic // This fixture speaks the agent protocol, not coderd API JSON.
	return json.NewDecoder(res.Body).Decode(output)
}

func (a *controllerHTTPAgent) ListProcesses(ctx context.Context) (out workspacesdk.ListProcessesResponse, err error) {
	err = a.request(ctx, http.MethodGet, "/process/list", nil, &out)
	return out, err
}

func (a *controllerHTTPAgent) StartProcess(ctx context.Context, in workspacesdk.StartProcessRequest) (out workspacesdk.StartProcessResponse, err error) {
	err = a.request(ctx, http.MethodPost, "/process/start-tracked", in, &out)
	return out, err
}

func (a *controllerHTTPAgent) ProcessOutput(ctx context.Context, id string, options *workspacesdk.ProcessOutputOptions) (out workspacesdk.ProcessOutputResponse, err error) {
	path := fmt.Sprintf("/process/%s/output-tracked?agent_instance_id=%s&wait=%t&wait_ms=%d", id, options.AgentInstanceID, options.Wait, options.WaitMillis)
	err = a.request(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (a *controllerHTTPAgent) CancelProcess(ctx context.Context, in workspacesdk.CancelProcessRequest) (out workspacesdk.CancelProcessResponse, err error) {
	err = a.request(ctx, http.MethodPost, "/process/cancel-tracked", in, &out)
	return out, err
}

func (a *controllerHTTPAgent) BundleFiles(ctx context.Context, in workspacesdk.BundleFilesRequest) (out []byte, err error) {
	if a.beforeBundle != nil {
		if err := a.beforeBundle(ctx); err != nil {
			return nil, err
		}
	}
	err = a.request(ctx, http.MethodPost, "/files/bundle-files", in, &out)
	return out, err
}

func newControllerFixture(t *testing.T) *controllerFixture {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, provisioner, api := coderdtest.NewWithAPI(t, &coderdtest.Options{Database: db, Pubsub: ps, IncludeProvisionerDaemon: true, ChatWorkerDisabled: true, Clock: quartz.NewMock(t)})
	first := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, first.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, first.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: workspace.LatestBuild.Job.ID, Transition: database.WorkspaceTransitionStart})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	processAPI := agentproc.NewAPI(testutil.Logger(t), agentexec.DefaultExecer, nil, nil, nil, nil, nil)
	t.Cleanup(func() { require.NoError(t, processAPI.Close()) })
	mux := http.NewServeMux()
	mux.Handle("/process/", http.StripPrefix("/process", processAPI.Routes()))
	mux.Handle("/files/", http.StripPrefix("/files", agentfiles.NewAPI(testutil.Logger(t), nil, nil).Routes()))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	conn := &controllerHTTPAgent{server: server}
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Time(time.Now().Add(time.Hour))).MustWait(t.Context())
	controller := workspaceexec.NewController(workspaceexec.ControllerOptions{
		Database: db, Clock: clock, Logger: testutil.Logger(t), Interval: time.Second,
		DialAgent: func(ctx context.Context, id uuid.UUID) (workspaceexec.ControllerAgent, func(), error) {
			if id != agent.ID {
				return nil, nil, xerrors.New("wrong agent")
			}
			return conn, nil, nil
		},
		FileCache: api.FileCache, UsageChecker: func() wsbuilder.UsageChecker { return *api.BuildUsageChecker.Load() },
		Pubsub: ps, DeploymentValues: api.DeploymentValues, Experiments: api.Experiments,
		AllowDeletion: true,
	})
	t.Cleanup(controller.Close)
	return &controllerFixture{api: api, provisioner: provisioner, db: db, sqlDB: sqlDB, client: client, workspace: workspace, agentID: agent.ID, agent: conn, clock: clock, controller: controller}
}

func (f *controllerFixture) session(t *testing.T, paths []string, deadline *time.Time) database.WorkspaceExecutionSession {
	t.Helper()
	declarations, err := json.Marshal(map[string]any{"version": 1, "result_paths": paths, "result_agent_id": f.agentID, "execution_deadline": deadline})
	require.NoError(t, err)
	id := uuid.New()
	_, err = f.sqlDB.ExecContext(t.Context(), `INSERT INTO workspace_execution_sessions
 (id,organization_id,owner_id,actor_id,request_id,input_digest,workspace_id,workspace_owner_id,
 created_at,updated_at,lease_expires_at,state,declarations,disposable,retained)
 VALUES($1,$2,$3,$3,$4,$5,$6,$3,$7,$7,$7::timestamptz + interval '1 second','active',$8,true,false)`,
		id, f.workspace.OrganizationID, f.workspace.OwnerID, uuid.New(), make([]byte, 32), f.workspace.ID, f.clock.Now(), declarations)
	require.NoError(t, err)
	session, err := f.db.GetWorkspaceExecutionSessionByID(t.Context(), id)
	require.NoError(t, err)
	return session
}

func (f *controllerFixture) tick(t *testing.T) {
	t.Helper()
	f.clock.Advance(time.Second).MustWait(t.Context())
}

func (f *controllerFixture) current(t *testing.T, id uuid.UUID) database.WorkspaceExecutionSession {
	t.Helper()
	session, err := f.db.GetWorkspaceExecutionSessionByID(t.Context(), id)
	require.NoError(t, err)
	return session
}
