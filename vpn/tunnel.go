package vpn

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/util/dnsname"
	"tailscale.com/wgengine/router"

	"cdr.dev/slog"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

// netStatusInterval is the interval at which the tunnel sends network status updates to the manager.
// This is currently only used to keep `last_handshake` up to date.
const netStatusInterval = 10 * time.Second

type Tunnel struct {
	speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	updater

	ctx             context.Context
	requestLoopDone chan struct{}

	logger slog.Logger

	logMu sync.Mutex
	logs  []*TunnelMessage

	client Client

	// clientLogger is a separate logger than `logger` when the `UseAsLogger`
	// option is used, to avoid the tunnel using itself as a sink for it's own
	// logs, which could lead to deadlocks.
	clientLogger slog.Logger
	// the following may be nil
	networkingStackFn func(*Tunnel, *StartRequest, slog.Logger) (NetworkStack, error)
}

type TunnelOption func(t *Tunnel)

func NewTunnel(
	ctx context.Context,
	logger slog.Logger,
	mgrConn io.ReadWriteCloser,
	client Client,
	opts ...TunnelOption,
) (*Tunnel, error) {
	logger = logger.Named("vpn")
	s, err := newSpeaker[*TunnelMessage, *ManagerMessage](
		ctx, logger, mgrConn, SpeakerRoleTunnel, SpeakerRoleManager)
	if err != nil {
		return nil, err
	}
	uCtx, uCancel := context.WithCancel(ctx)
	t := &Tunnel{
		//nolint:govet // safe to copy the locks here because we haven't started the speaker
		speaker:         *(s),
		ctx:             ctx,
		logger:          logger,
		clientLogger:    logger,
		requestLoopDone: make(chan struct{}),
		client:          client,
		updater: updater{
			ctx:         uCtx,
			cancel:      uCancel,
			netLoopDone: make(chan struct{}),
			uSendCh:     s.sendCh,
			agents:      map[uuid.UUID]tailnet.Agent{},
			clock:       quartz.NewReal(),
		},
	}

	for _, opt := range opts {
		opt(t)
	}
	t.speaker.start()
	go t.requestLoop()
	go t.netStatusLoop()
	return t, nil
}

func (t *Tunnel) requestLoop() {
	defer close(t.requestLoopDone)
	for req := range t.speaker.requests {
		if req.msg.Rpc != nil && req.msg.Rpc.MsgId != 0 {
			t.handleRPC(req)
			if _, ok := req.msg.GetMsg().(*ManagerMessage_Stop); ok {
				close(t.sendCh)
				return
			}
			continue
		}
		// Not a unary RPC. We don't know of any message types that are neither a response nor a
		// unary RPC from the Manager.  This shouldn't ever happen because we checked the protocol
		// version during the handshake.
		t.logger.Critical(t.ctx, "unknown request", slog.F("msg", req.msg))
	}
}

// handleRPC handles unary RPCs from the manager, sending a reply back to the manager.
func (t *Tunnel) handleRPC(req *request[*TunnelMessage, *ManagerMessage]) {
	resp := &TunnelMessage{}
	resp.Rpc = &RPC{ResponseTo: req.msg.Rpc.MsgId}
	switch msg := req.msg.GetMsg().(type) {
	case *ManagerMessage_GetPeerUpdate:
		err := t.updater.sendUpdateResponse(req)
		if err != nil {
			t.logger.Error(t.ctx, "failed to send peer update", slog.Error(err))
		}
		// Reply has already been sent.
		return
	case *ManagerMessage_Start:
		startReq := msg.Start
		t.logger.Info(t.ctx, "starting CoderVPN tunnel",
			slog.F("url", startReq.CoderUrl),
			slog.F("tunnel_fd", startReq.TunnelFileDescriptor),
		)
		err := t.start(startReq)
		var errStr string
		if err != nil {
			t.logger.Error(t.ctx, "failed to start tunnel", slog.Error(err))
			errStr = err.Error()
		}
		resp.Msg = &TunnelMessage_Start{
			Start: &StartResponse{
				Success:      err == nil,
				ErrorMessage: errStr,
			},
		}
	case *ManagerMessage_Stop:
		t.logger.Info(t.ctx, "stopping CoderVPN tunnel")
		err := t.stop(msg.Stop)
		var errStr string
		if err != nil {
			t.logger.Error(t.ctx, "failed to stop tunnel", slog.Error(err))
			errStr = err.Error()
		} else {
			t.logger.Info(t.ctx, "coderVPN tunnel stopped")
		}
		resp.Msg = &TunnelMessage_Stop{
			Stop: &StopResponse{
				Success:      err == nil,
				ErrorMessage: errStr,
			},
		}
	default:
		t.logger.Warn(t.ctx, "unhandled manager request", slog.F("request", msg))
	}
	if err := req.sendReply(resp); err != nil {
		t.logger.Debug(t.ctx, "failed to send RPC reply", slog.Error(err))
	}
}

type NetworkStack struct {
	WireguardMonitor *netmon.Monitor
	TUNDevice        tun.Device
	Router           router.Router
	DNSConfigurator  dns.OSConfigurator
}

func UseOSNetworkingStack() TunnelOption {
	return func(t *Tunnel) {
		t.networkingStackFn = GetNetworkingStack
	}
}

func UseAsLogger() TunnelOption {
	return func(t *Tunnel) {
		t.clientLogger = t.clientLogger.AppendSinks(t)
	}
}

func UseCustomLogSinks(sinks ...slog.Sink) TunnelOption {
	return func(t *Tunnel) {
		t.clientLogger = t.clientLogger.AppendSinks(sinks...)
	}
}

func WithClock(clock quartz.Clock) TunnelOption {
	return func(t *Tunnel) {
		t.clock = clock
	}
}

// ApplyNetworkSettings sends a request to the manager to apply the given network settings
func (t *Tunnel) ApplyNetworkSettings(ctx context.Context, ns *NetworkSettingsRequest) error {
	msg, err := t.speaker.unaryRPC(ctx, &TunnelMessage{
		Msg: &TunnelMessage_NetworkSettings{
			NetworkSettings: ns,
		},
	})
	if err != nil {
		return xerrors.Errorf("rpc failure: %w", err)
	}
	resp := msg.GetNetworkSettings()
	if !resp.Success {
		return xerrors.Errorf("network settings failed: %s", resp.ErrorMessage)
	}
	return nil
}

func (t *Tunnel) start(req *StartRequest) error {
	rawURL := req.GetCoderUrl()
	if rawURL == "" {
		return xerrors.New("missing coder url")
	}
	svrURL, err := url.Parse(rawURL)
	if err != nil {
		return xerrors.Errorf("parse url %q: %w", rawURL, err)
	}
	apiToken := req.GetApiToken()
	if apiToken == "" {
		return xerrors.New("missing api token")
	}
	header := make(http.Header)
	for _, h := range req.GetHeaders() {
		header.Add(h.GetName(), h.GetValue())
	}

	// Add desktop telemetry if any fields are provided
	telemetryData := codersdk.CoderDesktopTelemetry{
		DeviceID:            req.GetDeviceId(),
		DeviceOS:            req.GetDeviceOs(),
		CoderDesktopVersion: req.GetCoderDesktopVersion(),
	}
	if !telemetryData.IsEmpty() {
		headerValue, err := json.Marshal(telemetryData)
		if err == nil {
			header.Set(codersdk.CoderDesktopTelemetryHeader, string(headerValue))
			t.logger.Debug(t.ctx, "added desktop telemetry header",
				slog.F("data", telemetryData))
		} else {
			t.logger.Warn(t.ctx, "failed to marshal telemetry data")
		}
	}

	var networkingStack NetworkStack
	if t.networkingStackFn != nil {
		networkingStack, err = t.networkingStackFn(t, req, t.clientLogger)
		if err != nil {
			return xerrors.Errorf("failed to create networking stack dependencies: %w", err)
		}
	} else {
		t.logger.Debug(t.ctx, "using default networking stack as no custom stack was provided")
	}

	conn, err := t.client.NewConn(
		t.ctx,
		svrURL,
		apiToken,
		&Options{
			Headers:          header,
			Logger:           t.clientLogger,
			DNSConfigurator:  networkingStack.DNSConfigurator,
			Router:           networkingStack.Router,
			TUNDevice:        networkingStack.TUNDevice,
			WireguardMonitor: networkingStack.WireguardMonitor,
			UpdateHandler:    t,
		},
	)
	if err != nil {
		return xerrors.Errorf("failed to start connection: %w", err)
	}

	if ok := t.updater.setConn(conn); !ok {
		t.logger.Warn(t.ctx, "asked to start tunnel, but tunnel is already running")
	}
	return err
}

func (t *Tunnel) stop(*StopRequest) error {
	return t.updater.stop()
}

var _ slog.Sink = &Tunnel{}

func (t *Tunnel) LogEntry(_ context.Context, e slog.SinkEntry) {
	t.logMu.Lock()
	defer t.logMu.Unlock()
	t.logs = append(t.logs, &TunnelMessage{
		Msg: &TunnelMessage_Log{
			Log: sinkEntryToPb(e),
		},
	})
}

func (t *Tunnel) Sync() {
	t.logMu.Lock()
	logs := t.logs
	t.logs = nil
	t.logMu.Unlock()
	for _, msg := range logs {
		select {
		case <-t.ctx.Done():
			return
		case t.sendCh <- msg:
		}
	}
}

func sinkEntryToPb(e slog.SinkEntry) *Log {
	l := &Log{
		// #nosec G115 - Safe conversion for log levels which are small positive integers
		Level:       Log_Level(e.Level),
		Message:     e.Message,
		LoggerNames: e.LoggerNames,
	}
	for _, field := range e.Fields {
		l.Fields = append(l.Fields, &Log_Field{
			Name:  field.Name,
			Value: formatValue(field.Value),
		})
	}
	return l
}

// updater is the component of the tunnel responsible for sending workspace
// updates to the manager.
type updater struct {
	ctx         context.Context
	cancel      context.CancelFunc
	netLoopDone chan struct{}

	mu      sync.Mutex
	uSendCh chan<- *TunnelMessage
	// agents contains the agents that are currently connected to the tunnel.
	agents map[uuid.UUID]tailnet.Agent
	conn   Conn

	clock quartz.Clock
}

// Update pushes a workspace update to the manager
func (u *updater) Update(update tailnet.WorkspaceUpdate) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	peerUpdate := u.createPeerUpdateLocked(update)
	msg := &TunnelMessage{
		Msg: &TunnelMessage_PeerUpdate{
			PeerUpdate: peerUpdate,
		},
	}
	select {
	case <-u.ctx.Done():
		return u.ctx.Err()
	case u.uSendCh <- msg:
	}
	return nil
}

// sendUpdateResponse responds to the provided `ManagerMessage_GetPeerUpdate` request
// with the current state of the workspaces.
func (u *updater) sendUpdateResponse(req *request[*TunnelMessage, *ManagerMessage]) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	state, err := u.conn.CurrentWorkspaceState()
	if err != nil {
		return xerrors.Errorf("failed to get current workspace state: %w", err)
	}
	update := u.createPeerUpdateLocked(state)
	resp := &TunnelMessage{
		Msg: &TunnelMessage_PeerUpdate{
			PeerUpdate: update,
		},
	}
	err = req.sendReply(resp)
	if err != nil {
		return xerrors.Errorf("failed to send RPC reply: %w", err)
	}
	return nil
}

// createPeerUpdateLocked creates a PeerUpdate message from a workspace update, populating
// the network status of the agents.
func (u *updater) createPeerUpdateLocked(update tailnet.WorkspaceUpdate) *PeerUpdate {
	out := &PeerUpdate{
		UpsertedWorkspaces: make([]*Workspace, len(update.UpsertedWorkspaces)),
		UpsertedAgents:     make([]*Agent, len(update.UpsertedAgents)),
		DeletedWorkspaces:  make([]*Workspace, len(update.DeletedWorkspaces)),
		DeletedAgents:      make([]*Agent, len(update.DeletedAgents)),
	}

	u.saveUpdateLocked(update)

	for i, ws := range update.UpsertedWorkspaces {
		out.UpsertedWorkspaces[i] = &Workspace{
			Id:     tailnet.UUIDToByteSlice(ws.ID),
			Name:   ws.Name,
			Status: Workspace_Status(ws.Status),
		}
	}
	upsertedAgents := u.convertAgentsLocked(update.UpsertedAgents)
	out.UpsertedAgents = upsertedAgents
	for i, ws := range update.DeletedWorkspaces {
		out.DeletedWorkspaces[i] = &Workspace{
			Id:     tailnet.UUIDToByteSlice(ws.ID),
			Name:   ws.Name,
			Status: Workspace_Status(ws.Status),
		}
	}
	for i, agent := range update.DeletedAgents {
		fqdn := make([]string, 0, len(agent.Hosts))
		for name := range agent.Hosts {
			fqdn = append(fqdn, name.WithTrailingDot())
		}
		sort.Slice(fqdn, func(i, j int) bool {
			return len(fqdn[i]) < len(fqdn[j])
		})
		out.DeletedAgents[i] = &Agent{
			Id:            tailnet.UUIDToByteSlice(agent.ID),
			Name:          agent.Name,
			WorkspaceId:   tailnet.UUIDToByteSlice(agent.WorkspaceID),
			Fqdn:          fqdn,
			IpAddrs:       hostsToIPStrings(agent.Hosts),
			LastHandshake: nil,
		}
	}
	return out
}

// convertAgentsLocked takes a list of `tailnet.Agent` and converts them to proto agents.
// If there is an active connection, the last handshake time is populated.
func (u *updater) convertAgentsLocked(agents []*tailnet.Agent) []*Agent {
	out := make([]*Agent, 0, len(agents))

	for _, agent := range agents {
		fqdn := make([]string, 0, len(agent.Hosts))
		for name := range agent.Hosts {
			fqdn = append(fqdn, name.WithTrailingDot())
		}
		sort.Slice(fqdn, func(i, j int) bool {
			return len(fqdn[i]) < len(fqdn[j])
		})
		protoAgent := &Agent{
			Id:          tailnet.UUIDToByteSlice(agent.ID),
			Name:        agent.Name,
			WorkspaceId: tailnet.UUIDToByteSlice(agent.WorkspaceID),
			Fqdn:        fqdn,
			IpAddrs:     hostsToIPStrings(agent.Hosts),
		}
		if u.conn != nil {
			diags := u.conn.GetPeerDiagnostics(agent.ID)
			protoAgent.LastHandshake = timestamppb.New(diags.LastWireguardHandshake)
		}
		out = append(out, protoAgent)
	}

	return out
}

// saveUpdateLocked saves the workspace update to the tunnel's state, such that it can
// be used to populate automated peer updates.
func (u *updater) saveUpdateLocked(update tailnet.WorkspaceUpdate) {
	for _, agent := range update.UpsertedAgents {
		u.agents[agent.ID] = agent.Clone()
	}
	for _, agent := range update.DeletedAgents {
		delete(u.agents, agent.ID)
	}
}

// setConn sets the `conn` and returns false if there's already a connection set.
func (u *updater) setConn(conn Conn) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.conn != nil {
		return false
	}
	u.conn = conn
	return true
}

func (u *updater) stop() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.conn == nil {
		return nil
	}
	err := u.conn.Close()
	u.conn = nil
	u.cancel()
	return err
}

// sendAgentUpdate sends a peer update message to the manager with the current
// state of the agents, including the latest network status.
func (u *updater) sendAgentUpdate() {
	u.mu.Lock()
	defer u.mu.Unlock()

	agents := make([]*tailnet.Agent, 0, len(u.agents))
	for _, agent := range u.agents {
		agents = append(agents, &agent)
	}
	upsertedAgents := u.convertAgentsLocked(agents)
	if len(upsertedAgents) == 0 {
		return
	}

	msg := &TunnelMessage{
		Msg: &TunnelMessage_PeerUpdate{
			PeerUpdate: &PeerUpdate{
				UpsertedAgents: upsertedAgents,
			},
		},
	}

	select {
	case <-u.ctx.Done():
		return
	case u.uSendCh <- msg:
	}
}

func (u *updater) netStatusLoop() {
	ticker := u.clock.NewTicker(netStatusInterval)
	defer ticker.Stop()
	defer close(u.netLoopDone)
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
			u.sendAgentUpdate()
		}
	}
}

// hostsToIPStrings returns a slice of all unique IP addresses in the values
// of the given map.
func hostsToIPStrings(hosts map[dnsname.FQDN][]netip.Addr) []string {
	seen := make(map[netip.Addr]struct{})
	var result []string
	for _, inner := range hosts {
		for _, elem := range inner {
			if _, exists := seen[elem]; !exists {
				seen[elem] = struct{}{}
				result = append(result, elem.String())
			}
		}
	}
	return result
}

// the following are taken from sloghuman:

func formatValue(v interface{}) string {
	if vr, ok := v.(driver.Valuer); ok {
		var err error
		v, err = vr.Value()
		if err != nil {
			return fmt.Sprintf("error calling Value: %v", err)
		}
	}
	if v == nil {
		return "<nil>"
	}
	typ := reflect.TypeOf(v)
	switch typ.Kind() {
	case reflect.Struct, reflect.Map:
		byt, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		return string(byt)
	case reflect.Slice:
		// Byte slices are optimistically readable.
		if typ.Elem().Kind() == reflect.Uint8 {
			return fmt.Sprintf("%q", v)
		}
		fallthrough
	default:
		return quote(fmt.Sprintf("%+v", v))
	}
}

// quotes quotes a string so that it is suitable
// as a key for a map or in general some output that
// cannot span multiple lines or have weird characters.
func quote(key string) string {
	// strconv.Quote does not quote an empty string so we need this.
	if key == "" {
		return `""`
	}

	var hasSpace bool
	for _, r := range key {
		if unicode.IsSpace(r) {
			hasSpace = true
			break
		}
	}
	quoted := strconv.Quote(key)
	// If the key doesn't need to be quoted, don't quote it.
	// We do not use strconv.CanBackquote because it doesn't
	// account tabs.
	if !hasSpace && quoted[1:len(quoted)-1] == key {
		return key
	}
	return quoted
}
