package agentacp

import (
	"context"
	"maps"
	"path/filepath"
	"slices"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

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

type sessionRuntimeOptions struct {
	launch launchOptions
	fs     afero.Fs
	logger slog.Logger
}

// startSessionRuntime creates a native session and attaches its callbacks before
// publication.
func startSessionRuntime(ctx context.Context, cfg harnessConfig, req workspacesdk.ACPCreateSessionRequest, opts sessionRuntimeOptions) (*SessionFSM, error) {
	if err := validateSessionRequest(opts.fs, req.HarnessSlug, req.WorkingDirectory); err != nil {
		return nil, err
	}
	handler := &sessionClient{}
	p, err := launch(ctx, cfg, req.WorkingDirectory, handler, opts.launch)
	if err != nil {
		return nil, xerrors.Errorf("%w: %v", ErrUnavailable, err)
	}
	ok := false
	defer func() {
		if !ok {
			p.close()
		}
	}()
	setupCtx, cancel := timeout(ctx, opts.launch.clock, setupTimeout)
	defer cancel()
	init, err := p.conn.Initialize(setupCtx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientInfo: &acp.Implementation{Name: "coder-agent", Version: buildinfo.Version()}})
	if err != nil {
		return nil, xerrors.Errorf("%w: initialize harness: %v", ErrUnavailable, harnessFailure(ctx, opts.logger, p, err, cfg))
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber {
		return nil, xerrors.Errorf("%w: unsupported ACP protocol version", ErrUnavailable)
	}
	harness := harnessInfo(cfg, init)
	created, err := p.conn.NewSession(setupCtx, acp.NewSessionRequest{Cwd: req.WorkingDirectory, McpServers: []acp.McpServer{}})
	if err != nil {
		return nil, xerrors.Errorf("%w: create ACP session: %v", ErrUnavailable, err)
	}
	if created.SessionId == "" {
		return nil, xerrors.Errorf("%w: harness returned an empty session id", ErrUnavailable)
	}
	nativeID, options := string(created.SessionId), created.ConfigOptions
	for _, id := range slices.Sorted(maps.Keys(req.Config)) {
		value := req.Config[id]
		if !validConfig(options, id, value) {
			return nil, xerrors.Errorf("%w: unsupported configuration %q=%q", ErrInvalid, id, value)
		}
		updated, err := p.conn.SetSessionConfigOption(setupCtx, acp.SetSessionConfigOptionRequest{ValueId: &acp.SetSessionConfigOptionValueId{SessionId: acp.SessionId(nativeID), ConfigId: acp.SessionConfigId(id), Value: acp.SessionConfigValueId(value)}})
		if err != nil {
			return nil, xerrors.Errorf("%w: configure ACP session: %v", ErrInvalid, err)
		}
		options = updated.ConfigOptions
	}
	id := workspacesdk.ACPSessionID{HarnessSlug: req.HarnessSlug, WorkingDirectory: req.WorkingDirectory, SessionID: nativeID}
	info := workspacesdk.ACPSession{ID: id, Config: maps.Clone(req.Config), Status: workspacesdk.ACPSessionStatusIdle, Cursor: workspacesdk.ACPCursor{Epoch: uuid.New()}}
	info.Harness, info.HarnessDisplayName = harness, cfg.displayName
	info.Harness.ConfigOptions = configOptions(options)
	s := newSessionFSM(ctx, req, info, p, opts.logger)
	if err := handler.attachSession(s); err != nil {
		s.Close()
		return nil, err
	}
	ok = true
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
