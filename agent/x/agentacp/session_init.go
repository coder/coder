package agentacp

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const sessionInitializationTimeout = 30 * time.Second

func validateSessionRequest(fs afero.Fs, slug, directory string) error {
	if slug == "" {
		return xerrors.Errorf("%w: harness slug is required", ErrInvalid)
	}
	if directory == "" || !filepath.IsAbs(directory) {
		return xerrors.Errorf("%w: working_directory must be an explicit absolute path", ErrInvalid)
	}
	info, err := fs.Stat(directory)
	if err != nil || !info.IsDir() {
		return xerrors.Errorf("%w: working_directory must be an existing directory", ErrInvalid)
	}
	return nil
}

type sessionInitRequest struct {
	harnessSlug      string
	workingDirectory string
	nativeID         string // Empty creates a new native session.
	config           map[string]string
}

type sessionRuntimeOptions struct {
	launch launchOptions
	fs     afero.Fs
	logger slog.Logger
}

// startSessionRuntime creates an initializing FSM before launching the harness.
// Setup and history replay finish before the idle FSM is published.
func startSessionRuntime(ctx context.Context, cfg harnessConfig, req sessionInitRequest, opts sessionRuntimeOptions) (session *SessionFSM, err error) {
	if err := validateSessionRequest(opts.fs, req.harnessSlug, req.workingDirectory); err != nil {
		return nil, err
	}
	id := workspacesdk.ACPSessionID{
		HarnessSlug:      req.harnessSlug,
		WorkingDirectory: req.workingDirectory,
		SessionID:        req.nativeID,
	}
	info := workspacesdk.ACPSession{
		ID:                 id,
		Config:             maps.Clone(req.config),
		HarnessDisplayName: cfg.displayName,
		Status:             workspacesdk.ACPSessionStatusInitializing,
		Cursor: workspacesdk.ACPCursor{
			GenerationID: uuid.New(),
		},
	}
	s := newSessionFSM(ctx, req, info)
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	p, err := launch(s.ctx, cfg, req.workingDirectory, &sessionClient{session: s}, opts.launch)
	if err != nil {
		return nil, xerrors.Errorf("%w: %v", ErrUnavailable, err)
	}
	s.startProcess(p, opts.logger)
	// Close the session when its parent context is canceled.
	// Register after startProcess so Close cannot run before s.proc is set.
	context.AfterFunc(s.ctx, s.Close)
	setupCtx, cancel := timeout(ctx, opts.launch.clock, sessionInitializationTimeout)
	defer cancel()
	init, err := p.conn.Initialize(setupCtx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    "coder-agent",
			Version: buildinfo.Version(),
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("%w: initialize harness: %v", ErrUnavailable, harnessFailure(ctx, opts.logger, p, err, cfg))
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber {
		return nil, xerrors.Errorf("%w: unsupported ACP protocol version", ErrUnavailable)
	}
	nativeID := req.nativeID
	harness := harnessInfo(cfg, init)
	var options []acp.SessionConfigOption
	switch {
	case nativeID == "":
		created, err := p.conn.NewSession(setupCtx, acp.NewSessionRequest{Cwd: req.workingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: create ACP session: %v", ErrUnavailable, err)
		}
		if created.SessionId == "" {
			return nil, xerrors.Errorf("%w: harness returned an empty session id", ErrUnavailable)
		}
		nativeID, options = string(created.SessionId), created.ConfigOptions
	case harness.LoadSession:
		loaded, err := p.conn.LoadSession(setupCtx, acp.LoadSessionRequest{SessionId: acp.SessionId(nativeID), Cwd: req.workingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: load ACP session: %v", ErrUnavailable, err)
		}
		options = loaded.ConfigOptions
	case harness.ResumeSession:
		resumed, err := p.conn.ResumeSession(setupCtx, acp.ResumeSessionRequest{SessionId: acp.SessionId(nativeID), Cwd: req.workingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: resume ACP session: %v", ErrUnavailable, err)
		}
		options = resumed.ConfigOptions
	default:
		return nil, xerrors.Errorf("%w: harness does not support restoration", ErrUnavailable)
	}
	for _, id := range slices.Sorted(maps.Keys(req.config)) {
		value := req.config[id]
		if !validConfig(options, id, value) {
			return nil, xerrors.Errorf("%w: unsupported configuration %q=%q", ErrInvalid, id, value)
		}
		updated, err := p.conn.SetSessionConfigOption(setupCtx, acp.SetSessionConfigOptionRequest{
			ValueId: &acp.SetSessionConfigOptionValueId{
				SessionId: acp.SessionId(nativeID),
				ConfigId:  acp.SessionConfigId(id),
				Value:     acp.SessionConfigValueId(value),
			},
		})
		if err != nil {
			return nil, xerrors.Errorf("%w: configure ACP session: %v", ErrInvalid, err)
		}
		options = updated.ConfigOptions
	}
	harness.ConfigOptions = configOptions(options)
	if err := s.finishInitialization(nativeID, harness); err != nil {
		return nil, err
	}
	return s, nil
}

func validConfig(options []acp.SessionConfigOption, id, value string) bool {
	for _, opt := range configOptions(options) {
		if opt.ID == id {
			for _, v := range opt.Values {
				if v.ID == value {
					return true
				}
			}
		}
	}
	return false
}

func harnessFailure(ctx context.Context, logger slog.Logger, p *process, err error, cfg harnessConfig) error {
	logger.Warn(ctx, "acp harness failed", slog.F("harness", cfg.slug), slog.Error(err), slog.F("stderr", p.stderr.String()))
	return xerrors.New("harness failed; see workspace agent logs for details")
}
