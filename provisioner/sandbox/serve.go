// Package sandbox is a provisioner that builds a workspace without
// Terraform. A workspace of a sandbox template is one pod, claimed from a
// warm pool that the Kubernetes Agent Sandbox project (agents.x-k8s.io)
// keeps ready. Starting the workspace claims a pod, and stopping or
// deleting it gives the pod up.
//
// A pod from a warm pool already runs a workspace daemon when it is claimed,
// so the build cannot hand the daemon a token. The provisioner registers
// the pod by the UID the cluster gave it, and the daemon proves that it runs
// in that pod with the token the kubelet obtained for the pod.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/coderd/x/agentsandbox"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	// TemplateFile is the file a sandbox template consists of.
	TemplateFile = provisionersdk.SandboxTemplateFile
	// stateFile carries what the previous build left, and then what this
	// one leaves, between the steps of a build.
	stateFile = "sandbox-state.json"
	// claimTimeout bounds how long a claim may take. A claim on an empty
	// pool waits for a new pod to start.
	claimTimeout = 3 * time.Minute
)

// Template is the content of a sandbox template.
type Template struct {
	// WarmPool names the SandboxWarmPool that workspaces of this template
	// claim their pod from. The pool decides everything about the pod: its
	// image, its resources, its network and its isolation.
	WarmPool string `yaml:"warm_pool"`
	// OS and Arch describe the pods of the pool. They default to linux
	// and amd64.
	OS   string `yaml:"os"`
	Arch string `yaml:"arch"`
	// UserData lets the pod's daemon fetch its owner's Git SSH key and
	// external auth tokens, as the daemon of an ordinary workspace can.
	// It is off by default: a sandbox usually runs something that is not
	// its owner.
	UserData bool `yaml:"user_data"`
}

// state is the provisioner state of a workspace: its claim, if it has one.
type state struct {
	Namespace string `json:"namespace,omitempty"`
	WarmPool  string `json:"warm_pool,omitempty"`
	Claim     string `json:"claim,omitempty"`
	Sandbox   string `json:"sandbox,omitempty"`
	PodName   string `json:"pod_name,omitempty"`
	PodUID    string `json:"pod_uid,omitempty"`
	Warm      bool   `json:"warm,omitempty"`
}

// ServeOptions configures the provisioner.
type ServeOptions struct {
	*provisionersdk.ServeOptions
	// Kube reaches the cluster that runs the sandboxes.
	Kube *agentsandbox.KubeClient
	// Namespace is where sandboxes are claimed. Templates cannot choose
	// another.
	Namespace string
}

// Serve runs the provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.Kube == nil || options.Namespace == "" {
		return xerrors.New("the sandbox provisioner needs a cluster and a namespace")
	}
	return provisionersdk.Serve(ctx, &server{kube: options.Kube, namespace: options.Namespace}, options.ServeOptions)
}

type server struct {
	kube      *agentsandbox.KubeClient
	namespace string
}

// claimName is the name of a workspace's claim. A workspace has at most
// one, so that claiming twice yields the same pod.
func claimName(workspaceID string) string {
	return "coder-" + workspaceID
}

func readTemplate(sess *provisionersdk.Session) (Template, error) {
	raw, err := os.ReadFile(filepath.Join(sess.Files.WorkDirectory(), TemplateFile))
	if err != nil {
		return Template{}, xerrors.Errorf("a sandbox template consists of a file named %s: %w", TemplateFile, err)
	}
	var template Template
	if err := yaml.Unmarshal(raw, &template); err != nil {
		return Template{}, xerrors.Errorf("parse %s: %w", TemplateFile, err)
	}
	if template.WarmPool == "" {
		return Template{}, xerrors.Errorf("%s must name a warm_pool", TemplateFile)
	}
	if template.OS == "" {
		template.OS = "linux"
	}
	if template.Arch == "" {
		template.Arch = "amd64"
	}
	return template, nil
}

func readState(sess *provisionersdk.Session) (state, error) {
	var st state
	raw, err := os.ReadFile(filepath.Join(sess.Files.WorkDirectory(), stateFile))
	if err != nil {
		return st, err
	}
	if len(raw) == 0 {
		return st, nil
	}
	return st, json.Unmarshal(raw, &st)
}

func writeState(sess *provisionersdk.Session, raw []byte) error {
	return os.WriteFile(filepath.Join(sess.Files.WorkDirectory(), stateFile), raw, 0o600)
}

func (*server) Init(sess *provisionersdk.Session, req *provisionersdk.InitRequest, _ <-chan struct{}) *proto.InitComplete {
	err := sess.Files.ExtractArchive(sess.Context(), sess.Logger, afero.NewOsFs(), req.TemplateSourceArchive, nil)
	if err != nil {
		return provisionersdk.InitErrorf("extract archive: %s", err.Error())
	}
	if _, err := readTemplate(sess); err != nil {
		return provisionersdk.InitErrorf("%s", err.Error())
	}
	return &proto.InitComplete{}
}

// Parse finds nothing to configure: a sandbox template has no variables.
func (*server) Parse(*provisionersdk.Session, *proto.ParseRequest, <-chan struct{}) *proto.ParseComplete {
	return &proto.ParseComplete{}
}

// Plan changes nothing. It keeps what the previous build left for the
// steps that follow.
func (*server) Plan(sess *provisionersdk.Session, req *proto.PlanRequest, _ <-chan struct{}) *proto.PlanComplete {
	if err := writeState(sess, req.GetState()); err != nil {
		return provisionersdk.PlanErrorf("keep state: %s", err.Error())
	}
	// An empty plan, and not none: coderd keeps a template version's plan
	// to render its parameters from, and a sandbox template has none.
	return &proto.PlanComplete{Plan: []byte("{}")}
}

func (s *server) Apply(sess *provisionersdk.Session, req *proto.ApplyRequest, canceledOrComplete <-chan struct{}) *proto.ApplyComplete {
	template, err := readTemplate(sess)
	if err != nil {
		return provisionersdk.ApplyErrorf("%s", err.Error())
	}
	ctx, cancel := context.WithTimeout(sess.Context(), claimTimeout)
	defer cancel()
	go func() {
		select {
		case <-canceledOrComplete:
			cancel()
		case <-ctx.Done():
		}
	}()

	metadata := req.GetMetadata()
	claim := claimName(metadata.GetWorkspaceId())
	if metadata.GetWorkspaceTransition() != proto.WorkspaceTransition_START {
		// The pod of a previous build may have been claimed in another
		// namespace, if the deployment's setting changed since.
		namespace := s.namespace
		if previous, err := readState(sess); err == nil && previous.Namespace != "" {
			namespace = previous.Namespace
		}
		if err := s.kube.ReleaseSandbox(ctx, namespace, claim); err != nil {
			return provisionersdk.ApplyErrorf("%s", err.Error())
		}
		sess.ProvisionLog(proto.LogLevel_INFO, fmt.Sprintf("Gave up sandbox claim %s/%s. Its pod is deleted.", namespace, claim))
		if err := writeState(sess, nil); err != nil {
			return provisionersdk.ApplyErrorf("keep state: %s", err.Error())
		}
		return &proto.ApplyComplete{}
	}

	started := time.Now()
	pod, err := s.kube.ClaimSandbox(ctx, s.namespace, claim, template.WarmPool)
	if err != nil {
		return provisionersdk.ApplyErrorf("%s", err.Error())
	}
	how := "a pod that was started for it"
	if pod.Warm {
		how = "a pod that was already running"
	}
	sess.ProvisionLog(proto.LogLevel_INFO, fmt.Sprintf("Claimed sandbox %s/%s from pool %s in %s: %s.",
		pod.Namespace, pod.Sandbox, template.WarmPool, time.Since(started).Round(time.Millisecond), how))
	raw, err := json.Marshal(state{
		Namespace: pod.Namespace, WarmPool: template.WarmPool, Claim: pod.ClaimName,
		Sandbox: pod.Sandbox, PodName: pod.PodName, PodUID: pod.PodUID, Warm: pod.Warm,
	})
	if err != nil {
		return provisionersdk.ApplyErrorf("marshal state: %s", err.Error())
	}
	if err := writeState(sess, raw); err != nil {
		return provisionersdk.ApplyErrorf("keep state: %s", err.Error())
	}
	return &proto.ApplyComplete{State: raw}
}

// Graph reports the resources of the workspace: its pod with the daemon in
// it, or nothing when the workspace is stopped.
func (*server) Graph(sess *provisionersdk.Session, req *proto.GraphRequest, _ <-chan struct{}) *proto.GraphComplete {
	template, err := readTemplate(sess)
	if err != nil {
		return provisionersdk.GraphError("%s", err.Error())
	}
	if req.GetMetadata().GetWorkspaceTransition() != proto.WorkspaceTransition_START {
		return &proto.GraphComplete{}
	}

	agent := &proto.Agent{
		Name:                     "main",
		OperatingSystem:          template.OS,
		Architecture:             template.Arch,
		ConnectionTimeoutSeconds: 120,
		ApiKeyScope:              "no_user_data",
		DisplayApps:              &proto.DisplayApps{WebTerminal: true, SshHelper: true},
	}
	if template.UserData {
		agent.ApiKeyScope = "all"
	}
	resource := &proto.Resource{
		Name:   "pod",
		Type:   "agent_sandbox",
		Icon:   "/icon/k8s.png",
		Agents: []*proto.Agent{agent},
		Metadata: []*proto.Resource_Metadata{
			{Key: "warm_pool", Value: template.WarmPool},
		},
	}
	// What a build would make is all a template import or a dry run asks
	// for. Which pod it will be is known once a build has claimed one.
	if req.GetSource() == proto.GraphSource_SOURCE_STATE {
		st, err := readState(sess)
		if err != nil || st.PodUID == "" {
			return provisionersdk.GraphError("the workspace has no sandbox")
		}
		agent.Auth = &proto.Agent_InstanceId{InstanceId: agentsandbox.InstanceIDPrefix + st.PodUID}
		launch := "cold"
		if st.Warm {
			launch = "warm"
		}
		resource.Metadata = append(resource.Metadata,
			&proto.Resource_Metadata{Key: "namespace", Value: st.Namespace},
			&proto.Resource_Metadata{Key: "pod", Value: st.PodName},
			&proto.Resource_Metadata{Key: "pod_uid", Value: st.PodUID},
			&proto.Resource_Metadata{Key: "launch", Value: launch},
		)
	}
	return &proto.GraphComplete{Resources: []*proto.Resource{resource}}
}

func (*server) Shutdown(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}
