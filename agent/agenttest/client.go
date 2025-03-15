package agenttest

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	drpcsdk "github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

const statsInterval = 500 * time.Millisecond

func NewClient(t testing.TB,
	logger slog.Logger,
	agentID uuid.UUID,
	manifest agentsdk.Manifest,
	statsChan chan *agentproto.Stats,
	coordinator tailnet.Coordinator,
) *Client {
	if manifest.AgentID == uuid.Nil {
		manifest.AgentID = agentID
	}
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coordinator)
	mux := drpcmux.New()
	derpMapUpdates := make(chan *tailcfg.DERPMap)
	drpcService := &tailnet.DRPCService{
		CoordPtr:               &coordPtr,
		Logger:                 logger.Named("tailnetsvc"),
		DerpMapUpdateFrequency: time.Microsecond,
		DerpMapFn:              func() *tailcfg.DERPMap { return <-derpMapUpdates },
	}
	err := proto.DRPCRegisterTailnet(mux, drpcService)
	require.NoError(t, err)
	mp, err := agentsdk.ProtoFromManifest(manifest)
	require.NoError(t, err)
	fakeAAPI := NewFakeAgentAPI(t, logger, mp, statsChan)
	err = agentproto.DRPCRegisterAgent(mux, fakeAAPI)
	require.NoError(t, err)
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if errors.Is(err, io.EOF) {
				return
			}
			logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})
	return &Client{
		t:              t,
		logger:         logger.Named("client"),
		agentID:        agentID,
		server:         server,
		fakeAgentAPI:   fakeAAPI,
		derpMapUpdates: derpMapUpdates,
	}
}

type Client struct {
	t                  testing.TB
	logger             slog.Logger
	agentID            uuid.UUID
	server             *drpcserver.Server
	fakeAgentAPI       *FakeAgentAPI
	LastWorkspaceAgent func()

	mu             sync.Mutex // Protects following.
	logs           []agentsdk.Log
	derpMapUpdates chan *tailcfg.DERPMap
	derpMapOnce    sync.Once
}

func (*Client) RewriteDERPMap(*tailcfg.DERPMap) {}

func (c *Client) Close() {
	c.derpMapOnce.Do(func() { close(c.derpMapUpdates) })
}

func (c *Client) ConnectRPC24(ctx context.Context) (
	agentproto.DRPCAgentClient24, proto.DRPCTailnetClient24, error,
) {
	conn, lis := drpcsdk.MemTransportPipe()
	c.LastWorkspaceAgent = func() {
		_ = conn.Close()
		_ = lis.Close()
	}
	c.t.Cleanup(c.LastWorkspaceAgent)
	serveCtx, cancel := context.WithCancel(ctx)
	c.t.Cleanup(cancel)
	streamID := tailnet.StreamID{
		Name: "agenttest",
		ID:   c.agentID,
		Auth: tailnet.AgentCoordinateeAuth{ID: c.agentID},
	}
	serveCtx = tailnet.WithStreamID(serveCtx, streamID)
	go func() {
		_ = c.server.Serve(serveCtx, lis)
	}()
	return agentproto.NewDRPCAgentClient(conn), proto.NewDRPCTailnetClient(conn), nil
}

func (c *Client) GetLifecycleStates() []codersdk.WorkspaceAgentLifecycle {
	return c.fakeAgentAPI.GetLifecycleStates()
}

func (c *Client) GetStartup() <-chan *agentproto.Startup {
	return c.fakeAgentAPI.startupCh
}

func (c *Client) GetMetadata() map[string]agentsdk.Metadata {
	return c.fakeAgentAPI.GetMetadata()
}

func (c *Client) GetStartupLogs() []agentsdk.Log {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logs
}

func (c *Client) SetAnnouncementBannersFunc(f func() ([]codersdk.BannerConfig, error)) {
	c.fakeAgentAPI.SetAnnouncementBannersFunc(f)
}

func (c *Client) PushDERPMapUpdate(update *tailcfg.DERPMap) error {
	timer := time.NewTimer(testutil.WaitShort)
	defer timer.Stop()
	select {
	case c.derpMapUpdates <- update:
	case <-timer.C:
		return xerrors.New("timeout waiting to push derp map update")
	}

	return nil
}

func (c *Client) SetLogsChannel(ch chan<- *agentproto.BatchCreateLogsRequest) {
	c.fakeAgentAPI.SetLogsChannel(ch)
}

func (c *Client) GetConnectionReports() []*agentproto.ReportConnectionRequest {
	return c.fakeAgentAPI.GetConnectionReports()
}

type FakeAgentAPI struct {
	sync.Mutex
	t      testing.TB
	logger slog.Logger

	manifest          *agentproto.Manifest
	startupCh         chan *agentproto.Startup
	statsCh           chan *agentproto.Stats
	appHealthCh       chan *agentproto.BatchUpdateAppHealthRequest
	logsCh            chan<- *agentproto.BatchCreateLogsRequest
	lifecycleStates   []codersdk.WorkspaceAgentLifecycle
	metadata          map[string]agentsdk.Metadata
	timings           []*agentproto.Timing
	connectionReports []*agentproto.ReportConnectionRequest

	getAnnouncementBannersFunc              func() ([]codersdk.BannerConfig, error)
	getResourcesMonitoringConfigurationFunc func() (*agentproto.GetResourcesMonitoringConfigurationResponse, error)
	pushResourcesMonitoringUsageFunc        func(*agentproto.PushResourcesMonitoringUsageRequest) (*agentproto.PushResourcesMonitoringUsageResponse, error)
}

func (f *FakeAgentAPI) GetManifest(context.Context, *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	return f.manifest, nil
}

func (*FakeAgentAPI) GetServiceBanner(context.Context, *agentproto.GetServiceBannerRequest) (*agentproto.ServiceBanner, error) {
	return &agentproto.ServiceBanner{}, nil
}

func (f *FakeAgentAPI) GetTimings() []*agentproto.Timing {
	f.Lock()
	defer f.Unlock()
	return slices.Clone(f.timings)
}

func (f *FakeAgentAPI) SetAnnouncementBannersFunc(fn func() ([]codersdk.BannerConfig, error)) {
	f.Lock()
	defer f.Unlock()
	f.getAnnouncementBannersFunc = fn
	f.logger.Info(context.Background(), "updated notification banners")
}

func (f *FakeAgentAPI) GetAnnouncementBanners(context.Context, *agentproto.GetAnnouncementBannersRequest) (*agentproto.GetAnnouncementBannersResponse, error) {
	f.Lock()
	defer f.Unlock()
	if f.getAnnouncementBannersFunc == nil {
		return &agentproto.GetAnnouncementBannersResponse{AnnouncementBanners: []*agentproto.BannerConfig{}}, nil
	}
	banners, err := f.getAnnouncementBannersFunc()
	if err != nil {
		return nil, err
	}
	bannersProto := make([]*agentproto.BannerConfig, 0, len(banners))
	for _, banner := range banners {
		bannersProto = append(bannersProto, agentsdk.ProtoFromBannerConfig(banner))
	}
	return &agentproto.GetAnnouncementBannersResponse{AnnouncementBanners: bannersProto}, nil
}

func (f *FakeAgentAPI) GetResourcesMonitoringConfiguration(_ context.Context, _ *agentproto.GetResourcesMonitoringConfigurationRequest) (*agentproto.GetResourcesMonitoringConfigurationResponse, error) {
	f.Lock()
	defer f.Unlock()

	if f.getResourcesMonitoringConfigurationFunc == nil {
		return &agentproto.GetResourcesMonitoringConfigurationResponse{
			Config: &agentproto.GetResourcesMonitoringConfigurationResponse_Config{
				CollectionIntervalSeconds: 10,
				NumDatapoints:             20,
			},
		}, nil
	}

	return f.getResourcesMonitoringConfigurationFunc()
}

func (f *FakeAgentAPI) PushResourcesMonitoringUsage(_ context.Context, req *agentproto.PushResourcesMonitoringUsageRequest) (*agentproto.PushResourcesMonitoringUsageResponse, error) {
	f.Lock()
	defer f.Unlock()

	if f.pushResourcesMonitoringUsageFunc == nil {
		return &agentproto.PushResourcesMonitoringUsageResponse{}, nil
	}

	return f.pushResourcesMonitoringUsageFunc(req)
}

func (f *FakeAgentAPI) UpdateStats(ctx context.Context, req *agentproto.UpdateStatsRequest) (*agentproto.UpdateStatsResponse, error) {
	f.logger.Debug(ctx, "update stats called", slog.F("req", req))
	// empty request is sent to get the interval; but our tests don't want empty stats requests
	if req.Stats != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case f.statsCh <- req.Stats:
			// OK!
		}
	}
	return &agentproto.UpdateStatsResponse{ReportInterval: durationpb.New(statsInterval)}, nil
}

func (f *FakeAgentAPI) GetLifecycleStates() []codersdk.WorkspaceAgentLifecycle {
	f.Lock()
	defer f.Unlock()
	return slices.Clone(f.lifecycleStates)
}

func (f *FakeAgentAPI) UpdateLifecycle(_ context.Context, req *agentproto.UpdateLifecycleRequest) (*agentproto.Lifecycle, error) {
	f.Lock()
	defer f.Unlock()
	s, err := agentsdk.LifecycleStateFromProto(req.GetLifecycle().GetState())
	if assert.NoError(f.t, err) {
		f.lifecycleStates = append(f.lifecycleStates, s)
	}
	return req.GetLifecycle(), nil
}

func (f *FakeAgentAPI) BatchUpdateAppHealths(ctx context.Context, req *agentproto.BatchUpdateAppHealthRequest) (*agentproto.BatchUpdateAppHealthResponse, error) {
	f.logger.Debug(ctx, "batch update app health", slog.F("req", req))
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.appHealthCh <- req:
		return &agentproto.BatchUpdateAppHealthResponse{}, nil
	}
}

func (f *FakeAgentAPI) AppHealthCh() <-chan *agentproto.BatchUpdateAppHealthRequest {
	return f.appHealthCh
}

func (f *FakeAgentAPI) UpdateStartup(ctx context.Context, req *agentproto.UpdateStartupRequest) (*agentproto.Startup, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.startupCh <- req.GetStartup():
		return req.GetStartup(), nil
	}
}

func (f *FakeAgentAPI) GetMetadata() map[string]agentsdk.Metadata {
	f.Lock()
	defer f.Unlock()
	return maps.Clone(f.metadata)
}

func (f *FakeAgentAPI) BatchUpdateMetadata(ctx context.Context, req *agentproto.BatchUpdateMetadataRequest) (*agentproto.BatchUpdateMetadataResponse, error) {
	f.Lock()
	defer f.Unlock()
	if f.metadata == nil {
		f.metadata = make(map[string]agentsdk.Metadata)
	}
	for _, md := range req.Metadata {
		smd := agentsdk.MetadataFromProto(md)
		f.metadata[md.Key] = smd
		f.logger.Debug(ctx, "post metadata", slog.F("key", md.Key), slog.F("md", md))
	}
	return &agentproto.BatchUpdateMetadataResponse{}, nil
}

func (f *FakeAgentAPI) SetLogsChannel(ch chan<- *agentproto.BatchCreateLogsRequest) {
	f.Lock()
	defer f.Unlock()
	f.logsCh = ch
}

func (f *FakeAgentAPI) BatchCreateLogs(ctx context.Context, req *agentproto.BatchCreateLogsRequest) (*agentproto.BatchCreateLogsResponse, error) {
	f.logger.Info(ctx, "batch create logs called", slog.F("req", req))
	f.Lock()
	ch := f.logsCh
	f.Unlock()
	if ch != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case ch <- req:
			// ok
		}
	}
	return &agentproto.BatchCreateLogsResponse{}, nil
}

func (f *FakeAgentAPI) ScriptCompleted(_ context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	f.Lock()
	f.timings = append(f.timings, req.GetTiming())
	f.Unlock()

	return &agentproto.WorkspaceAgentScriptCompletedResponse{}, nil
}

func (f *FakeAgentAPI) ReportConnection(_ context.Context, req *agentproto.ReportConnectionRequest) (*emptypb.Empty, error) {
	f.Lock()
	f.connectionReports = append(f.connectionReports, req)
	f.Unlock()

	return &emptypb.Empty{}, nil
}

func (f *FakeAgentAPI) GetConnectionReports() []*agentproto.ReportConnectionRequest {
	f.Lock()
	defer f.Unlock()
	return slices.Clone(f.connectionReports)
}

func NewFakeAgentAPI(t testing.TB, logger slog.Logger, manifest *agentproto.Manifest, statsCh chan *agentproto.Stats) *FakeAgentAPI {
	return &FakeAgentAPI{
		t:           t,
		logger:      logger.Named("FakeAgentAPI"),
		manifest:    manifest,
		statsCh:     statsCh,
		startupCh:   make(chan *agentproto.Startup, 100),
		appHealthCh: make(chan *agentproto.BatchUpdateAppHealthRequest, 100),
	}
}
