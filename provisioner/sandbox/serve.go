package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// ServeOptions configures the native provisioner. Runtime is injectable for
// tests; production connections are opened only when an apply needs compute.
type ServeOptions struct {
	*provisionersdk.ServeOptions
	Runtime        Runtime
	StateDirectory string
}

// Serve implements Coder's provisioner protocol without Terraform.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options == nil {
		options = &ServeOptions{}
	}
	s := &server{options: *options}
	defer func() {
		if s.ownedRuntime != nil {
			_ = s.ownedRuntime.Close()
		}
	}()
	return provisionersdk.Serve(ctx, s, options.ServeOptions)
}

type server struct {
	options      ServeOptions
	mu           sync.Mutex
	engine       *Engine
	ownedRuntime Runtime
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (s *server) getEngine() (*Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil {
		return s.engine, nil
	}
	directory := s.options.StateDirectory
	if directory == "" {
		directory = envDefault("CODER_SANDBOX_STATE_DIRECTORY", "/var/lib/coder-sandbox")
	}
	runtime := s.options.Runtime
	if runtime == nil {
		var err error
		runtime, err = NewRuntime(RuntimeOptions{
			Address:   envDefault("CODER_SANDBOX_CONTAINERD_ADDRESS", "/run/containerd/containerd.sock"),
			Namespace: "coder-sandbox", StateDirectory: directory,
			CNIConfigDir: envDefault("CODER_SANDBOX_CNI_CONFIG_DIRECTORY", "/etc/cni/net.d"),
			CNIBinDir:    envDefault("CODER_SANDBOX_CNI_BIN_DIRECTORY", "/opt/cni/bin"),
		})
		if err != nil {
			return nil, err
		}
	}
	engine, err := NewEngine(runtime, directory)
	if err != nil {
		if s.options.Runtime == nil {
			_ = runtime.Close()
		}
		return nil, err
	}
	s.engine = engine
	if s.options.Runtime == nil {
		s.ownedRuntime = runtime
	}
	return engine, nil
}

type sessionPlan struct {
	Manifest Manifest        `json:"manifest"`
	Metadata *proto.Metadata `json:"metadata"`
}

func sessionFile(sess *provisionersdk.Session, name string) string {
	return filepath.Join(sess.Files.WorkDirectory(), name)
}

func loadJSON(sess *provisionersdk.Session, name string, value any) error {
	data, err := os.ReadFile(sessionFile(sess, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func saveJSON(sess *provisionersdk.Session, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFile(sess, name), data, 0o600)
}

func timing(start time.Time, stage, action string, err error) []*proto.Timing {
	state := proto.TimingState_COMPLETED
	if err != nil {
		state = proto.TimingState_FAILED
	}
	return []*proto.Timing{{Start: timestamppb.New(start), End: timestamppb.Now(), Stage: stage, Source: "sandbox", Resource: "sandbox", Action: action, State: state}}
}

func requestContext(sess *provisionersdk.Session, done <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(sess.Context())
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (*server) Init(sess *provisionersdk.Session, req *provisionersdk.InitRequest, _ <-chan struct{}) *proto.InitComplete {
	start := time.Now()
	m, err := ReadManifestArchive(req.TemplateSourceArchive)
	if err == nil {
		err = os.MkdirAll(sess.Files.WorkDirectory(), 0o700)
	}
	if err == nil {
		err = saveJSON(sess, "manifest.json", m)
	}
	if err == nil {
		reader := tar.NewReader(bytes.NewReader(req.TemplateSourceArchive))
		for {
			header, readErr := reader.Next()
			if readErr != nil {
				break
			}
			if strings.TrimPrefix(header.Name, "./") == "README.md" && header.Typeflag == tar.TypeReg && header.Size <= 64<<10 {
				body, readErr := io.ReadAll(reader)
				if readErr == nil {
					err = os.WriteFile(sess.Files.ReadmeFilePath(), body, 0o600)
				}
				break
			}
		}
	}
	resp := &proto.InitComplete{Timings: timing(start, "init", "validate manifest", err)}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func (*server) Parse(sess *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	var m Manifest
	if err := loadJSON(sess, "manifest.json", &m); err != nil {
		return &proto.ParseComplete{Error: err.Error()}
	}
	return &proto.ParseComplete{WorkspaceTags: m.Tags()}
}

func (*server) Plan(sess *provisionersdk.Session, req *proto.PlanRequest, _ <-chan struct{}) *proto.PlanComplete {
	start := time.Now()
	resp := &proto.PlanComplete{}
	err := func() error {
		if req.Metadata == nil {
			return xerrors.New("sandbox plan requires metadata")
		}
		if req.Metadata.WorkspaceTransition != proto.WorkspaceTransition_START && req.Metadata.WorkspaceTransition != proto.WorkspaceTransition_STOP && req.Metadata.WorkspaceTransition != proto.WorkspaceTransition_DESTROY {
			return xerrors.New("unsupported sandbox transition")
		}
		if len(req.VariableValues) != 0 || len(req.RichParameterValues) != 0 {
			return xerrors.New("sandbox templates have fixed parameters")
		}
		if req.Metadata.GetPrebuiltWorkspaceBuildStage() != proto.PrebuiltWorkspaceBuildStage_NONE {
			return xerrors.New("sandbox provisioner does not support prebuilds")
		}
		if _, err := DecodeState(req.State, req.Metadata.WorkspaceId); err != nil {
			return err
		}
		var m Manifest
		if err := loadJSON(sess, "manifest.json", &m); err != nil {
			return err
		}
		if req.Metadata.WorkspaceTransition == proto.WorkspaceTransition_START {
			resp.DailyCost = m.DailyCost
		}
		return saveJSON(sess, "native-plan.json", sessionPlan{Manifest: m, Metadata: req.Metadata})
	}()
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Timings = timing(start, "plan", "plan lifecycle", err)
	return resp
}

func (s *server) Apply(sess *provisionersdk.Session, req *proto.ApplyRequest, done <-chan struct{}) *proto.ApplyComplete {
	start := time.Now()
	ctx, cancel := requestContext(sess, done)
	defer cancel()
	resp := &proto.ApplyComplete{}
	var runtimeTimings []*proto.Timing
	err := func() error {
		var plan sessionPlan
		if err := loadJSON(sess, "native-plan.json", &plan); err != nil {
			return err
		}
		m := req.Metadata
		if m == nil || plan.Metadata == nil || m.WorkspaceId != plan.Metadata.WorkspaceId || m.WorkspaceBuildId != plan.Metadata.WorkspaceBuildId || m.WorkspaceBuildNumber != plan.Metadata.WorkspaceBuildNumber || m.WorkspaceTransition != plan.Metadata.WorkspaceTransition || m.CoderUrl != plan.Metadata.CoderUrl {
			return xerrors.New("sandbox apply must match its plan")
		}
		if m.WorkspaceTransition == proto.WorkspaceTransition_START {
			u, err := url.Parse(m.CoderUrl)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
				return xerrors.New("sandbox requires an HTTP(S) Coder URL")
			}
		}
		engine, err := s.getEngine()
		if err != nil {
			return err
		}
		state, applyErr := engine.execute(ctx, plan.Manifest, m, &runtimeTimings)
		if state.Version != 0 {
			resp.State, err = json.Marshal(state)
			if err != nil {
				return err
			}
			if err := saveJSON(sess, "native-state.json", state); err != nil {
				return err
			}
		}
		return applyErr
	}()
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Timings = append(timing(start, "apply", "runtime lifecycle", err), runtimeTimings...)
	return resp
}

func (*server) Graph(sess *provisionersdk.Session, req *proto.GraphRequest, _ <-chan struct{}) *proto.GraphComplete {
	start := time.Now()
	resp := &proto.GraphComplete{}
	err := func() error {
		var m Manifest
		var token string
		var transition proto.WorkspaceTransition
		switch req.Source {
		case proto.GraphSource_SOURCE_PLAN:
			var plan sessionPlan
			if err := loadJSON(sess, "native-plan.json", &plan); err != nil {
				return err
			}
			m, transition = plan.Manifest, plan.Metadata.WorkspaceTransition
		case proto.GraphSource_SOURCE_STATE:
			var state State
			if err := loadJSON(sess, "native-state.json", &state); err != nil {
				return err
			}
			m, token, transition = state.Manifest, state.AgentToken, state.Transition
			if state.Phase != "running" {
				return nil
			}
		default:
			return xerrors.New("unknown sandbox graph source")
		}
		if transition != proto.WorkspaceTransition_START {
			return nil
		}
		resp.Resources = []*proto.Resource{{
			Name: "sandbox", Type: "sandbox_container", DailyCost: m.DailyCost,
			InstanceType: fmt.Sprintf("%g CPU / %d MiB", m.CPU, m.MemoryMiB),
			Agents: []*proto.Agent{{
				Name: "main", OperatingSystem: "linux", Architecture: "amd64", Directory: m.Workdir,
				Auth: &proto.Agent_Token{Token: token}, ConnectionTimeoutSeconds: 30,
				DisplayApps: &proto.DisplayApps{Vscode: true, WebTerminal: true, SshHelper: true, PortForwardingHelper: true},
			}},
		}}
		return nil
	}()
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Timings = timing(start, "graph", "describe resources", err)
	return resp
}

var _ provisionersdk.Server = (*server)(nil)
