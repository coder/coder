//go:build linux

package sandbox

import (
	"bytes"
	"context"
	_ "crypto/sha256" // Register the supported image digest algorithm.
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	runtimeoptions "github.com/containerd/containerd/api/types/runtimeoptions/v1"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	cni "github.com/containerd/go-cni"
	"github.com/distribution/reference"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

const (
	sandboxNamespace   = "coder-sandbox"
	sandboxRuntime     = "io.containerd.runsc.v1"
	sandboxSnapshotter = "overlayfs"
	managedLabel       = "com.coder.sandbox.managed"
	workspaceLabel     = "com.coder.sandbox.workspace-id"
	buildLabel         = "com.coder.sandbox.build-id"
	buildNumberLabel   = "com.coder.sandbox.build-number"
	imageLabel         = "com.coder.sandbox.image"
	sandboxRunscConfig = "[runsc_config]\nplatform = \"systrap\"\n"
	sandboxRunscRoot   = "/run/containerd/runsc/coder-sandbox"
)

type runtimeClient interface {
	GetImage(context.Context, string) (containerd.Image, error)
	LoadContainer(context.Context, string) (containerd.Container, error)
	NewContainer(context.Context, string, ...containerd.NewContainerOpts) (containerd.Container, error)
	Containers(context.Context, ...string) ([]containerd.Container, error)
	SnapshotService(string) snapshots.Snapshotter
	Close() error
}

type containerdRuntime struct {
	client           runtimeClient
	networkDirectory string
	networkConfig    string
	runscConfigPath  string
	newNetwork       func(string) (cni.CNI, error)
	createNamespace  func(string) error
	removeNamespace  func(string) error
	removeRunsc      func(context.Context, RuntimeAllocation) (bool, error)
}

// networkOwner is written before any network or runtime allocation. Keeping the
// original configuration permits CNI DEL after an operator changes the config.
type networkOwner struct {
	Allocation RuntimeAllocation `json:"allocation"`
	Config     string            `json:"config"`
}

// NewRuntime connects to a host containerd configured with the runsc shim.
// Images must already be present and unpacked in the coder-sandbox namespace.
func NewRuntime(options RuntimeOptions) (Runtime, error) {
	if options.Namespace != "" && options.Namespace != sandboxNamespace {
		return nil, xerrors.Errorf("sandbox runtime namespace must be %q", sandboxNamespace)
	}
	if options.Address == "" {
		options.Address = "/run/containerd/containerd.sock"
	}
	if options.CNIConfigDir == "" {
		options.CNIConfigDir = "/etc/cni/net.d"
	}
	if options.CNIBinDir == "" {
		options.CNIBinDir = "/opt/cni/bin"
	}
	if !filepath.IsAbs(options.StateDirectory) {
		return nil, xerrors.New("sandbox runtime state directory must be absolute")
	}
	for _, dir := range []string{options.StateDirectory, filepath.Join(options.StateDirectory, "network")} {
		if err := privateDirectory(dir); err != nil {
			return nil, err
		}
	}
	runscConfigPath := filepath.Join(options.StateDirectory, "runsc.toml")
	if err := ensureRunscConfig(runscConfigPath); err != nil {
		return nil, err
	}
	runscBinary, err := exec.LookPath("runsc")
	if err != nil {
		return nil, xerrors.Errorf("resolve absolute runsc binary for verified cleanup: %w", err)
	}
	if !filepath.IsAbs(runscBinary) {
		return nil, xerrors.New("runsc cleanup binary must resolve to an absolute path")
	}
	configuration, err := cni.New(cni.WithPluginConfDir(options.CNIConfigDir), cni.WithDefaultConf)
	if err != nil {
		return nil, xerrors.Errorf("load sandbox CNI configuration: %w", err)
	}
	networks := configuration.GetConfig().Networks
	if len(networks) != 1 || len(networks[0].Config.Plugins) == 0 || networks[0].Config.Plugins[0].Network.Type != "bridge" {
		return nil, xerrors.New("sandbox CNI configuration must begin with a bridge plugin")
	}
	client, err := containerd.New(options.Address, containerd.WithDefaultNamespace(sandboxNamespace), containerd.WithTimeout(5*time.Second))
	if err != nil {
		return nil, xerrors.Errorf("connect to sandbox containerd: %w", err)
	}
	return &containerdRuntime{
		client:           client,
		networkDirectory: filepath.Join(options.StateDirectory, "network"),
		networkConfig:    networks[0].Config.Source,
		runscConfigPath:  runscConfigPath,
		newNetwork: func(config string) (cni.CNI, error) {
			return cni.New(cni.WithPluginDir([]string{options.CNIBinDir}), cni.WithConfListBytes([]byte(config)), cni.WithLoNetwork)
		},
		createNamespace: createNetworkNamespace,
		removeNamespace: removeNetworkNamespace,
		removeRunsc: func(ctx context.Context, allocation RuntimeAllocation) (bool, error) {
			return removeRunscAllocation(ctx, runscBinary, allocation)
		},
	}, nil
}

func (r *containerdRuntime) Close() error { return r.client.Close() }

func (r *containerdRuntime) Start(ctx context.Context, spec RuntimeSpec) error {
	if err := validateRuntimeSpec(spec); err != nil {
		return err
	}
	ctx = namespaces.WithNamespace(ctx, sandboxNamespace)
	allocation := RuntimeAllocation{ID: spec.ID, WorkspaceID: spec.WorkspaceID, BuildID: spec.BuildID, BuildNumber: spec.BuildNumber, Image: spec.Image}
	owner, err := r.readOwner(spec.ID)
	if err != nil {
		return err
	}
	if owner != nil && owner.Allocation != allocation {
		return xerrors.Errorf("%w: requested identity differs from existing network allocation", ErrRuntimeOwnership)
	}
	container, err := r.client.LoadContainer(ctx, spec.ID)
	if err == nil {
		found, err := ownedContainer(ctx, container)
		if err != nil {
			return err
		}
		if found != allocation {
			return xerrors.Errorf("%w: requested identity differs from existing container", ErrRuntimeOwnership)
		}
		running, err := r.Inspect(ctx, spec.ID)
		if err != nil {
			return err
		}
		if !running {
			return xerrors.New("existing sandbox task is not running; delete the allocation before retrying")
		}
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return xerrors.Errorf("find sandbox container: %w", err)
	}
	image, err := r.client.GetImage(ctx, spec.Image)
	if err != nil {
		return xerrors.Errorf("sandbox image must be preloaded in containerd namespace %q: %w", sandboxNamespace, err)
	}
	named, err := reference.ParseNormalizedNamed(spec.Image)
	if err != nil {
		return xerrors.Errorf("parse sandbox image reference: %w", err)
	}
	canonical, ok := named.(reference.Canonical)
	if !ok || image.Target().Digest != canonical.Digest() {
		return xerrors.New("preloaded sandbox image does not match its requested digest")
	}
	unpacked, err := image.IsUnpacked(ctx, sandboxSnapshotter)
	if err != nil {
		return xerrors.Errorf("inspect sandbox image unpack state: %w", err)
	}
	if !unpacked {
		return xerrors.New("sandbox image must be unpacked with the overlayfs snapshotter before starting")
	}

	newOwner := networkOwner{Allocation: allocation, Config: r.networkConfig}
	if err := r.writeOwner(newOwner); err != nil {
		return err
	}
	dir := filepath.Join(r.networkDirectory, spec.ID)
	netns := filepath.Join(dir, "netns")
	if err := r.createNamespace(netns); err != nil {
		return xerrors.Errorf("create sandbox network namespace: %w", err)
	}
	network, err := r.newNetwork(newOwner.Config)
	if err != nil {
		return xerrors.Errorf("load sandbox network: %w", err)
	}
	result, err := network.SetupSerially(ctx, spec.ID, netns)
	if err != nil {
		return xerrors.Errorf("configure sandbox network: %w", err)
	}
	resolver, err := sandboxResolver(result)
	if err != nil {
		return err
	}
	// These nonsecret files are mounted read-only and must be readable by the sandbox's user.
	//nolint:gosec // The containing host directory is private; sandbox users need read permission.
	if err := os.WriteFile(filepath.Join(dir, "resolv.conf"), []byte(resolver), 0o644); err != nil {
		return xerrors.Errorf("write sandbox resolver: %w", err)
	}
	//nolint:gosec // The containing host directory is private; sandbox users need read permission.
	if err := os.WriteFile(filepath.Join(dir, "hosts"), []byte("127.0.0.1 localhost\n::1 localhost\n"), 0o644); err != nil {
		return xerrors.Errorf("write sandbox hosts: %w", err)
	}
	labels := allocationLabels(allocation)
	container, err = r.client.NewContainer(ctx, spec.ID,
		containerd.WithSnapshotter(sandboxSnapshotter),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(spec.ID, image, snapshots.WithLabels(labels)),
		containerd.WithRuntime(sandboxRuntime, &runtimeoptions.Options{
			TypeUrl: "io.containerd.runsc.v1.options", ConfigPath: r.runscConfigPath,
		}),
		containerd.WithContainerLabels(labels),
		containerd.WithNewSpec(append([]oci.SpecOpts{oci.WithImageConfig(image)}, sandboxSpecOptions(spec, dir)...)...),
	)
	if err != nil {
		return xerrors.Errorf("create sandbox container: %w", err)
	}
	// The agent reports through Coder once connected. NullIO avoids tying the
	// sandbox lifetime to this provisioner process's streams or file descriptors.
	if err := markTaskCreatePending(dir); err != nil {
		return err
	}
	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		return xerrors.Errorf("create runsc sandbox task: %w", err)
	}
	if err := os.Remove(filepath.Join(dir, "task-create-pending")); err != nil {
		return xerrors.Errorf("resolve task creation intent: %w", err)
	}
	if err := task.Start(ctx); err != nil {
		return xerrors.Errorf("start runsc sandbox task: %w", err)
	}
	return nil
}

func (r *containerdRuntime) Inspect(ctx context.Context, id string) (bool, error) {
	if err := validateAllocationID(id); err != nil {
		return false, err
	}
	ctx = namespaces.WithNamespace(ctx, sandboxNamespace)
	container, err := r.client.LoadContainer(ctx, id)
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("find sandbox container: %w", err)
	}
	allocation, err := ownedContainer(ctx, container)
	if err != nil {
		return false, err
	}
	owner, err := r.readOwner(id)
	if err != nil {
		return false, err
	}
	if owner == nil || owner.Allocation != allocation {
		return false, xerrors.Errorf("%w: container and network ownership differ", ErrRuntimeOwnership)
	}
	task, err := container.Task(ctx, nil)
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("find sandbox task: %w", err)
	}
	status, err := task.Status(ctx)
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("inspect sandbox task: %w", err)
	}
	return status.Status == containerd.Running, nil
}

func (r *containerdRuntime) Delete(ctx context.Context, id string) error {
	if err := validateAllocationID(id); err != nil {
		return err
	}
	ctx = namespaces.WithNamespace(ctx, sandboxNamespace)
	owner, err := r.readOwner(id)
	if err != nil {
		return err
	}
	container, err := r.client.LoadContainer(ctx, id)
	if err != nil && !errdefs.IsNotFound(err) {
		return xerrors.Errorf("find sandbox container for deletion: %w", err)
	}
	if errdefs.IsNotFound(err) {
		container = nil
	}
	var allocation RuntimeAllocation
	if owner != nil {
		allocation = owner.Allocation
	}
	if container != nil {
		found, err := ownedContainer(ctx, container)
		if err != nil {
			return err
		}
		if owner == nil || found != allocation {
			return xerrors.Errorf("%w: container and network ownership differ", ErrRuntimeOwnership)
		}
	}
	snapshotter := r.client.SnapshotService(sandboxSnapshotter)
	snapshot, err := snapshotter.Stat(ctx, id)
	if err != nil && !errdefs.IsNotFound(err) {
		return xerrors.Errorf("inspect sandbox snapshot before deletion: %w", err)
	}
	hasSnapshot := err == nil
	if hasSnapshot {
		found, err := allocationFromLabels(id, snapshot.Labels)
		if err != nil {
			return err
		}
		if owner != nil && found != allocation {
			return xerrors.Errorf("%w: snapshot and network ownership differ", ErrRuntimeOwnership)
		}
	}
	// Do not remove a running task's network or rootfs if termination failed.
	confirmedTaskDelete := false
	if container != nil {
		task, err := container.Task(ctx, nil)
		if err != nil && !errdefs.IsNotFound(err) {
			return xerrors.Errorf("find sandbox task for deletion: %w", err)
		}
		if err == nil {
			_, deleteErr := task.Delete(ctx, containerd.WithProcessKill)
			if deleteErr != nil && !errdefs.IsNotFound(deleteErr) {
				return xerrors.Errorf("delete sandbox task: %w", deleteErr)
			}
			confirmedTaskDelete = deleteErr == nil && task.Pid() != 0
		}
	}
	if owner != nil {
		pending, err := taskCreatePending(filepath.Join(r.networkDirectory, id))
		if err != nil {
			return err
		}
		removed, err := r.removeRunsc(ctx, allocation)
		if err != nil {
			return err
		}
		if pending && !removed && !confirmedTaskDelete {
			return xerrors.New("task creation outcome remains uncertain; retain ownership and retry cleanup when the runtime outcome is known")
		}
	}
	if container != nil {
		if err := container.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
			return xerrors.Errorf("delete sandbox container: %w", err)
		}
	}
	var cleanupErrors []error
	if hasSnapshot {
		if err := snapshotter.Remove(ctx, id); err != nil && !errdefs.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, xerrors.Errorf("delete sandbox snapshot: %w", err))
		}
	}
	if owner != nil {
		dir := filepath.Join(r.networkDirectory, id)
		netns := filepath.Join(dir, "netns")
		network, err := r.newNetwork(owner.Config)
		if err != nil {
			return errors.Join(append(cleanupErrors, xerrors.Errorf("load sandbox network for deletion: %w", err))...)
		}
		// A reboot removes the namespace bind mount but can leave its file.
		// CNI DEL with an empty path still releases IPAM allocations.
		path, err := liveNetworkNamespace(netns)
		if err != nil {
			return errors.Join(append(cleanupErrors, err)...)
		}
		if err := network.Remove(ctx, id, path); err != nil {
			return errors.Join(append(cleanupErrors, xerrors.Errorf("delete sandbox CNI network: %w", err))...)
		}
		if err := r.removeNamespace(netns); err != nil {
			cleanupErrors = append(cleanupErrors, xerrors.Errorf("remove sandbox network namespace: %w", err))
		}
		if len(cleanupErrors) == 0 {
			if err := os.RemoveAll(dir); err != nil {
				return xerrors.Errorf("remove sandbox network state: %w", err)
			}
		}
	} else {
		// No runtime or network resources are allocated until owner.json is
		// committed. A crash before then can leave only this staging file.
		// Remove that exact file and an empty directory, never unknown contents.
		if err := removeIncompleteNetworkState(filepath.Join(r.networkDirectory, id)); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func removeIncompleteNetworkState(directory string) error {
	partial := filepath.Join(directory, "owner.partial")
	info, err := os.Lstat(partial)
	if err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("inspect incomplete sandbox ownership: %w", err)
	}
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return xerrors.Errorf("%w: incomplete network ownership is not a private regular file", ErrRuntimeOwnership)
		}
		if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
			return xerrors.Errorf("remove incomplete sandbox ownership: %w", err)
		}
	}
	if err := os.Remove(directory); err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("remove empty sandbox network directory: %w", err)
	}
	return nil
}

func (r *containerdRuntime) List(ctx context.Context, workspaceID string) ([]RuntimeAllocation, error) {
	if err := canonicalUUID(workspaceID); err != nil {
		return nil, xerrors.Errorf("invalid workspace ID: %w", err)
	}
	ctx = namespaces.WithNamespace(ctx, sandboxNamespace)
	containers, err := r.client.Containers(ctx, fmt.Sprintf("labels.%q==%q,labels.%q==%q", managedLabel, "true", workspaceLabel, workspaceID))
	if err != nil {
		return nil, xerrors.Errorf("list sandbox containers: %w", err)
	}
	allocations := map[string]RuntimeAllocation{}
	for _, container := range containers {
		allocation, err := ownedContainer(ctx, container)
		if err != nil {
			return nil, err
		}
		if allocation.WorkspaceID != workspaceID {
			return nil, xerrors.Errorf("%w: containerd returned another workspace's sandbox", ErrRuntimeOwnership)
		}
		allocations[allocation.ID] = allocation
	}
	entries, err := os.ReadDir(r.networkDirectory)
	if err != nil {
		return nil, xerrors.Errorf("list sandbox network allocations: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "coder-sandbox-") {
			continue
		}
		owner, err := r.readOwner(entry.Name())
		if err != nil {
			return nil, err
		}
		if owner == nil || owner.Allocation.WorkspaceID != workspaceID {
			continue
		}
		if found, ok := allocations[owner.Allocation.ID]; ok && found != owner.Allocation {
			return nil, xerrors.Errorf("%w: container and network ownership differ", ErrRuntimeOwnership)
		}
		allocations[owner.Allocation.ID] = owner.Allocation
	}
	result := make([]RuntimeAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		result = append(result, allocation)
	}
	slices.SortFunc(result, func(a, b RuntimeAllocation) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

func sandboxSpecOptions(spec RuntimeSpec, directory string) []oci.SpecOpts {
	annotations := allocationLabels(RuntimeAllocation{ID: spec.ID, WorkspaceID: spec.WorkspaceID, BuildID: spec.BuildID, BuildNumber: spec.BuildNumber, Image: spec.Image})
	annotations["io.kubernetes.cri.container-type"] = "sandbox"
	return []oci.SpecOpts{
		// The runsc shim only attaches detached task I/O to an explicitly
		// annotated sandbox. Otherwise its create command captures output
		// through pipes inherited by the gofer and sentry, blocking creation.
		oci.WithAnnotations(annotations),
		oci.WithProcessArgs("/usr/local/bin/coder", "agent"),
		oci.WithEnv([]string{"CODER_AGENT_TOKEN=" + spec.AgentToken, "CODER_AGENT_URL=" + spec.CoderURL, "CODER_AGENT_AUTH=token"}),
		oci.WithProcessCwd(spec.Workdir),
		oci.WithHostname(spec.ID),
		oci.WithLinuxNamespace(specs.LinuxNamespace{Type: specs.NetworkNamespace, Path: filepath.Join(directory, "netns")}),
		oci.WithCapabilities(nil),
		oci.WithNoNewPrivileges,
		oci.WithMemoryLimit(uint64(spec.MemoryBytes)), //nolint:gosec // validateRuntimeSpec requires positive int64 memory before generating the OCI spec.
		oci.WithCPUCFS(int64(math.Ceil(spec.CPU*100000)), 100000),
		oci.WithPidsLimit(4096),
		oci.WithMounts([]specs.Mount{
			{Destination: "/etc/resolv.conf", Type: "bind", Source: filepath.Join(directory, "resolv.conf"), Options: []string{"bind", "ro", "nosuid", "nodev", "noexec"}},
			{Destination: "/etc/hosts", Type: "bind", Source: filepath.Join(directory, "hosts"), Options: []string{"bind", "ro", "nosuid", "nodev", "noexec"}},
		}),
	}
}

func validateRuntimeSpec(spec RuntimeSpec) error {
	if err := validateAllocation(RuntimeAllocation{ID: spec.ID, WorkspaceID: spec.WorkspaceID, BuildID: spec.BuildID, BuildNumber: spec.BuildNumber, Image: spec.Image}); err != nil {
		return err
	}
	if math.IsNaN(spec.CPU) || math.IsInf(spec.CPU, 0) || spec.CPU < 0.01 || spec.CPU > 1024 {
		return xerrors.New("sandbox CPU must be between 0.01 and 1024 cores")
	}
	if spec.MemoryBytes <= 0 {
		return xerrors.New("sandbox memory limit must be positive")
	}
	if !filepath.IsAbs(spec.Workdir) || filepath.Clean(spec.Workdir) != spec.Workdir {
		return xerrors.New("sandbox workdir must be an absolute clean path")
	}
	if spec.AgentToken == "" || strings.ContainsAny(spec.AgentToken, "\x00\r\n") {
		return xerrors.New("sandbox agent token must be nonempty and contain no control characters")
	}
	endpoint, err := url.Parse(spec.CoderURL)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return xerrors.New("sandbox Coder URL must be an absolute HTTP or HTTPS URL")
	}
	return nil
}

func validateAllocation(allocation RuntimeAllocation) error {
	if allocation.BuildNumber <= 0 {
		return xerrors.New("sandbox build number must be positive")
	}
	if err := canonicalUUID(allocation.WorkspaceID); err != nil {
		return xerrors.Errorf("invalid sandbox workspace ID: %w", err)
	}
	if err := canonicalUUID(allocation.BuildID); err != nil {
		return xerrors.Errorf("invalid sandbox build ID: %w", err)
	}
	if allocation.ID != "coder-sandbox-"+allocation.BuildID {
		return xerrors.New("sandbox allocation ID does not match its build")
	}
	named, err := reference.ParseNormalizedNamed(allocation.Image)
	if err != nil {
		return xerrors.Errorf("invalid sandbox image reference: %w", err)
	}
	canonical, ok := named.(reference.Canonical)
	if !ok || canonical.Digest().Algorithm() != digest.SHA256 {
		return xerrors.New("sandbox image must be pinned by a sha256 digest")
	}
	return nil
}

func validateAllocationID(id string) error {
	buildID, found := strings.CutPrefix(id, "coder-sandbox-")
	if !found {
		return xerrors.New("invalid sandbox allocation ID")
	}
	if err := canonicalUUID(buildID); err != nil {
		return xerrors.Errorf("invalid sandbox allocation ID: %w", err)
	}
	return nil
}

func canonicalUUID(value string) error {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return xerrors.New("expected a canonical, nonzero UUID")
	}
	return nil
}

func allocationLabels(allocation RuntimeAllocation) map[string]string {
	return map[string]string{managedLabel: "true", workspaceLabel: allocation.WorkspaceID, buildLabel: allocation.BuildID, buildNumberLabel: strconv.FormatInt(int64(allocation.BuildNumber), 10), imageLabel: allocation.Image}
}

func allocationFromLabels(id string, labels map[string]string) (RuntimeAllocation, error) {
	allocation := RuntimeAllocation{ID: id, WorkspaceID: labels[workspaceLabel], BuildID: labels[buildLabel], Image: labels[imageLabel]}
	if labels[managedLabel] != "true" {
		return RuntimeAllocation{}, xerrors.Errorf("%w: allocation is not managed by Coder sandboxes", ErrRuntimeOwnership)
	}
	buildNumber, err := strconv.ParseInt(labels[buildNumberLabel], 10, 32)
	if err != nil || buildNumber <= 0 || strconv.FormatInt(buildNumber, 10) != labels[buildNumberLabel] {
		return RuntimeAllocation{}, xerrors.Errorf("%w: invalid build number label", ErrRuntimeOwnership)
	}
	allocation.BuildNumber = int32(buildNumber)
	if err := validateAllocation(allocation); err != nil {
		return RuntimeAllocation{}, xerrors.Errorf("%w: invalid ownership labels: %v", ErrRuntimeOwnership, err)
	}
	return allocation, nil
}

func ownedContainer(ctx context.Context, container containerd.Container) (RuntimeAllocation, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return RuntimeAllocation{}, xerrors.Errorf("inspect sandbox container ownership: %w", err)
	}
	if info.Runtime.Name != sandboxRuntime || info.Snapshotter != sandboxSnapshotter || info.SnapshotKey != container.ID() {
		return RuntimeAllocation{}, xerrors.Errorf("%w: container runtime or snapshot differs", ErrRuntimeOwnership)
	}
	allocation, err := allocationFromLabels(container.ID(), info.Labels)
	if err != nil {
		return RuntimeAllocation{}, err
	}
	if info.Image != allocation.Image {
		return RuntimeAllocation{}, xerrors.Errorf("%w: container image differs from its labels", ErrRuntimeOwnership)
	}
	return allocation, nil
}

func privateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return xerrors.Errorf("create sandbox state directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return xerrors.Errorf("inspect sandbox state directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return xerrors.New("sandbox state directory must be a private directory without group or other permissions")
	}
	return nil
}

// The shim reads this path after the provisioner returns. Publish the complete
// fixed configuration atomically, without replacing an operator's existing file.
func ensureRunscConfig(path string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".runsc-*.toml")
	if err != nil {
		return xerrors.Errorf("create sandbox runsc configuration: %w", err)
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.WriteString(sandboxRunscConfig)
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	if err := errors.Join(writeErr, temporary.Close()); err != nil {
		return xerrors.Errorf("persist sandbox runsc configuration: %w", err)
	}
	if err := os.Link(temporary.Name(), path); err != nil && !os.IsExist(err) {
		return xerrors.Errorf("publish sandbox runsc configuration: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return xerrors.Errorf("inspect sandbox runsc configuration: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return xerrors.New("sandbox runsc configuration must be a regular file with mode 0600")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("read sandbox runsc configuration: %w", err)
	}
	if string(data) != sandboxRunscConfig {
		return xerrors.New("sandbox runsc configuration differs from the required systrap configuration")
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return xerrors.Errorf("open sandbox runsc configuration directory: %w", err)
	}
	if err := errors.Join(directory.Sync(), directory.Close()); err != nil {
		return xerrors.Errorf("sync sandbox runsc configuration directory: %w", err)
	}
	return nil
}

func (r *containerdRuntime) writeOwner(owner networkOwner) error {
	dir := filepath.Join(r.networkDirectory, owner.Allocation.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		if os.IsExist(err) {
			found, readErr := r.readOwner(owner.Allocation.ID)
			if readErr != nil {
				return readErr
			}
			if found == nil || found.Allocation != owner.Allocation {
				return xerrors.Errorf("%w: existing network allocation differs", ErrRuntimeOwnership)
			}
		}
		return xerrors.Errorf("create sandbox network state; delete incomplete allocations before retrying: %w", err)
	}
	committed := false
	defer func() {
		// This function has not created a namespace or a container yet.
		if !committed {
			_ = os.RemoveAll(dir)
		}
	}()
	data, err := json.Marshal(owner)
	if err != nil {
		return xerrors.Errorf("encode sandbox network ownership: %w", err)
	}
	temporary := filepath.Join(dir, "owner.partial")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return xerrors.Errorf("create sandbox network ownership: %w", err)
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return xerrors.Errorf("persist sandbox network ownership: %w", err)
	}
	if err := os.Rename(temporary, filepath.Join(dir, "owner.json")); err != nil {
		return xerrors.Errorf("publish sandbox network ownership: %w", err)
	}
	for _, directory := range []string{dir, r.networkDirectory} {
		file, err := os.Open(directory)
		if err != nil {
			return err
		}
		err = errors.Join(file.Sync(), file.Close())
		if err != nil {
			return xerrors.Errorf("sync sandbox network ownership directory: %w", err)
		}
	}
	committed = true
	return nil
}

//nolint:nilnil // Missing metadata is expected before allocation and after cleanup; callers distinguish it from invalid ownership.
func (r *containerdRuntime) readOwner(id string) (*networkOwner, error) {
	if err := validateAllocationID(id); err != nil {
		return nil, err
	}
	dir := filepath.Join(r.networkDirectory, id)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("inspect sandbox network directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, xerrors.Errorf("%w: network state is not a private directory", ErrRuntimeOwnership)
	}
	path := filepath.Join(dir, "owner.json")
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("inspect sandbox network ownership: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, xerrors.Errorf("%w: network ownership must be a private regular file", ErrRuntimeOwnership)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Inventory can overlap cleanup of a different workspace.
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("read sandbox network ownership: %w", err)
	}
	var owner networkOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		return nil, xerrors.Errorf("%w: decode network ownership: %v", ErrRuntimeOwnership, err)
	}
	if err := validateAllocation(owner.Allocation); err != nil {
		return nil, xerrors.Errorf("%w: validate network ownership: %v", ErrRuntimeOwnership, err)
	}
	if owner.Allocation.ID != id || owner.Config == "" {
		return nil, xerrors.Errorf("%w: network ownership does not match its allocation", ErrRuntimeOwnership)
	}
	return &owner, nil
}

func sandboxResolver(result *cni.Result) (string, error) {
	var servers []string
	if result != nil {
		for _, dns := range result.DNS {
			for _, value := range dns.Nameservers {
				address, err := netip.ParseAddr(value)
				if err != nil || address.IsLoopback() || address.IsUnspecified() {
					continue
				}
				if !slices.Contains(servers, address.String()) {
					servers = append(servers, address.String())
				}
			}
		}
	}
	if len(servers) == 0 {
		return "", xerrors.New("sandbox CNI must provide DNS nameservers reachable from the sandbox, not a host loopback resolver")
	}
	var resolver strings.Builder
	for _, server := range servers {
		_, _ = fmt.Fprintf(&resolver, "nameserver %s\n", server) // strings.Builder writes never fail.
	}
	return resolver.String(), nil
}

// A canceled Create RPC can return before containerd finishes its shim cleanup.
// Retain durable evidence of that uncertainty until a concrete runtime deletion.
type taskCreateIntent struct {
	Version int    `json:"version"`
	BootID  string `json:"boot_id"`
}

func currentBootID() (string, error) {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", xerrors.Errorf("read host boot identity: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if err := canonicalUUID(id); err != nil {
		return "", xerrors.Errorf("invalid host boot identity: %w", err)
	}
	return id, nil
}

func markTaskCreatePending(directory string) error {
	bootID, err := currentBootID()
	if err != nil {
		return err
	}
	data, err := json.Marshal(taskCreateIntent{Version: 1, BootID: bootID})
	if err != nil {
		return err
	}
	path := filepath.Join(directory, "task-create-pending")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return xerrors.Errorf("persist task creation intent: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return xerrors.Errorf("sync task creation intent: %w", err)
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func taskCreatePending(directory string) (bool, error) {
	path := filepath.Join(directory, "task-create-pending")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("inspect task creation intent: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > 512 {
		return false, xerrors.Errorf("%w: task creation intent is not a private regular record", ErrRuntimeOwnership)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, xerrors.Errorf("read task creation intent: %w", err)
	}
	var intent taskCreateIntent
	if err := json.Unmarshal(data, &intent); err != nil || intent.Version != 1 || canonicalUUID(intent.BootID) != nil {
		return false, xerrors.Errorf("%w: invalid versioned task creation intent", ErrRuntimeOwnership)
	}
	bootID, err := currentBootID()
	if err != nil {
		return false, err
	}
	// A host reboot terminates every old creator. The caller must still verify
	// runsc absence before releasing resources from a previous boot.
	return intent.BootID == bootID, nil
}

// containerd can lose its task record while an OCI runtime allocation survives.
// Verify the independent runtime identity before force-deleting only that ID.
func removeRunscAllocation(ctx context.Context, binary string, allocation RuntimeAllocation) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := func(args ...string) ([]byte, error) {
		// The binary is resolved once from the administrator's PATH; no shell is used.
		//nolint:gosec // Absolute trusted runtime binary and fixed/validated arguments.
		cmd := exec.CommandContext(ctx, binary, append([]string{"--root=" + sandboxRunscRoot, "--platform=systrap"}, args...)...)
		cmd.WaitDelay = time.Second
		var output, diagnostics bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &diagnostics
		if err := cmd.Run(); err != nil {
			return nil, xerrors.Errorf("runsc %s failed; retaining allocation ownership: %w", args[0], err)
		}
		// runsc list can skip unreadable state while emitting a warning. Such a
		// listing cannot establish absence, so preserve the journal for recovery.
		if diagnostics.Len() != 0 {
			return nil, xerrors.Errorf("runsc %s emitted diagnostics; retaining allocation ownership", args[0])
		}
		return output.Bytes(), nil
	}
	find := func() (*specs.State, error) {
		data, err := command("list", "--format=json")
		if err != nil {
			return nil, err
		}
		return runscAllocationState(data, allocation)
	}
	state, err := find()
	if err != nil || state == nil {
		return false, err
	}
	if _, err := command("delete", "--force", allocation.ID); err != nil {
		return false, err
	}
	state, err = find()
	if err != nil {
		return false, err
	}
	if state != nil {
		return false, xerrors.New("runsc allocation remains after deletion; retaining ownership")
	}
	return true, nil
}

//nolint:nilnil // A successfully read inventory with no matching ID means absent.
func runscAllocationState(data []byte, allocation RuntimeAllocation) (*specs.State, error) {
	var states []specs.State
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, xerrors.Errorf("decode runsc allocation inventory: %w", err)
	}
	var found *specs.State
	for i := range states {
		state := &states[i]
		if state.ID != allocation.ID {
			continue
		}
		identity, err := allocationFromLabels(state.ID, state.Annotations)
		if err != nil || identity != allocation || found != nil {
			return nil, xerrors.Errorf("%w: runsc allocation identity differs from durable ownership", ErrRuntimeOwnership)
		}
		found = state
	}
	return found, nil
}

func createNetworkNamespace(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		// A fresh locked thread is discarded on return if restoring its original
		// namespace fails. Never return a changed thread to Go's thread pool.
		runtime.LockOSThread()
		original, err := os.Open("/proc/thread-self/ns/net")
		if err != nil {
			runtime.UnlockOSThread()
			result <- err
			return
		}
		defer original.Close()
		if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
			runtime.UnlockOSThread()
			result <- err
			return
		}
		mountErr := unix.Mount("/proc/thread-self/ns/net", path, "none", unix.MS_BIND, "")
		restoreErr := unix.Setns(int(original.Fd()), unix.CLONE_NEWNET)
		if restoreErr == nil {
			runtime.UnlockOSThread()
		}
		result <- errors.Join(mountErr, restoreErr)
	}()
	return <-result
}

func liveNetworkNamespace(path string) (string, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return "", nil
		}
		return "", xerrors.Errorf("inspect sandbox namespace mount: %w", err)
	}
	if stat.Type != unix.NSFS_MAGIC {
		return "", nil
	}
	return path, nil
}

func removeNetworkNamespace(path string) error {
	if err := unix.Unmount(path, unix.MNT_DETACH); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Ensure the interface remains compatible with containerd's client.
var (
	_ runtimeClient = (*containerd.Client)(nil)
	_ Runtime       = (*containerdRuntime)(nil)
)
