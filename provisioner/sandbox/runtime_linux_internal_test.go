//go:build linux

package sandbox

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	cni "github.com/containerd/go-cni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func runtimeTestSpec() RuntimeSpec {
	build := uuid.NewString()
	return RuntimeSpec{
		ID: "coder-sandbox-" + build, WorkspaceID: uuid.NewString(), BuildID: build,
		BuildNumber: 1,
		Image:       "example.com/coder/sandbox@sha256:" + strings.Repeat("a", 64),
		CPU:         1.5, MemoryBytes: 2 << 30, Workdir: "/workspace",
		AgentToken: uuid.NewString(), CoderURL: "https://coder.example.com",
	}
}

func runtimeTestAllocation(spec RuntimeSpec) RuntimeAllocation {
	return RuntimeAllocation{ID: spec.ID, WorkspaceID: spec.WorkspaceID, BuildID: spec.BuildID, BuildNumber: spec.BuildNumber, Image: spec.Image}
}

func TestSandboxRuntimeRunscConfigConcurrentCreation(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "runsc.toml")
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for range cap(results) {
		workers.Go(func() { results <- ensureRunscConfig(path) })
	}
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "[runsc_config]\nplatform = \"systrap\"\n", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary configurations must be removed")
}

func TestSandboxRuntimeRunscConfigRejectsOverrides(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"other platform", "public file", "symlink"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			path := filepath.Join(directory, "runsc.toml")
			switch name {
			case "other platform":
				require.NoError(t, os.WriteFile(path, []byte("[runsc_config]\nplatform = \"ptrace\"\n"), 0o600))
			case "public file":
				//nolint:gosec // This fixture verifies that public permissions are rejected.
				require.NoError(t, os.WriteFile(path, []byte(sandboxRunscConfig), 0o644))
			case "symlink":
				target := filepath.Join(directory, "other.toml")
				require.NoError(t, os.WriteFile(target, []byte(sandboxRunscConfig), 0o600))
				require.NoError(t, os.Symlink(target, path))
			}
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Error(t, ensureRunscConfig(path))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSandboxRuntimeSpecIsolation(t *testing.T) {
	t.Parallel()
	input := runtimeTestSpec()
	directory := t.TempDir()
	ctx := namespaces.WithNamespace(t.Context(), sandboxNamespace)
	spec, err := oci.GenerateSpec(ctx, nil, &containers.Container{ID: input.ID}, sandboxSpecOptions(input, directory)...)
	require.NoError(t, err)
	// runsc's shim requires this annotation to wire NullIO into create.
	// An unspecified root container captures the long-lived sentry's pipes.
	require.Equal(t, "sandbox", spec.Annotations["io.kubernetes.cri.container-type"])
	for key, value := range allocationLabels(runtimeTestAllocation(input)) {
		require.Equal(t, value, spec.Annotations[key], "runtime cleanup must verify independent OCI ownership")
	}
	require.Equal(t, []string{"/usr/local/bin/coder", "agent"}, spec.Process.Args)
	require.Equal(t, input.Workdir, spec.Process.Cwd)
	require.True(t, spec.Process.NoNewPrivileges)
	require.Empty(t, spec.Process.Capabilities.Bounding)
	require.Empty(t, spec.Process.Capabilities.Effective)
	require.Empty(t, spec.Process.Capabilities.Permitted)
	require.Empty(t, spec.Process.Capabilities.Ambient)
	require.Equal(t, int64(150000), *spec.Linux.Resources.CPU.Quota)
	require.Equal(t, uint64(100000), *spec.Linux.Resources.CPU.Period)
	require.Equal(t, input.MemoryBytes, *spec.Linux.Resources.Memory.Limit)
	require.Equal(t, int64(4096), *spec.Linux.Resources.Pids.Limit)
	require.Contains(t, spec.Process.Env, "CODER_AGENT_TOKEN="+input.AgentToken)
	require.Contains(t, spec.Process.Env, "CODER_AGENT_AUTH=token")
	require.Contains(t, spec.Process.Env, "CODER_AGENT_URL="+input.CoderURL)
	var networkFound bool
	for _, namespace := range spec.Linux.Namespaces {
		if namespace.Type == specs.NetworkNamespace {
			require.Equal(t, filepath.Join(directory, "netns"), namespace.Path)
			networkFound = true
		} else {
			require.Empty(t, namespace.Path, "other namespaces must be newly created")
		}
	}
	require.True(t, networkFound)
	for _, mount := range spec.Mounts {
		if mount.Type != "bind" {
			continue
		}
		require.Contains(t, []string{filepath.Join(directory, "hosts"), filepath.Join(directory, "resolv.conf")}, mount.Source)
		require.Contains(t, mount.Options, "ro")
		require.Contains(t, mount.Options, "nodev")
		require.Contains(t, mount.Options, "nosuid")
		require.Contains(t, mount.Options, "noexec")
	}
}

func TestSandboxRuntimeRejectsInvalidSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*RuntimeSpec)
	}{
		{"path traversal", func(s *RuntimeSpec) { s.ID = "../../foreign" }},
		{"foreign build", func(s *RuntimeSpec) { s.BuildID = uuid.NewString() }},
		{"zero build number", func(s *RuntimeSpec) { s.BuildNumber = 0 }},
		{"zero workspace", func(s *RuntimeSpec) { s.WorkspaceID = uuid.Nil.String() }},
		{"mutable image", func(s *RuntimeSpec) { s.Image = "example.com/image:latest" }},
		{"nan cpu", func(s *RuntimeSpec) { s.CPU = math.NaN() }},
		{"infinite cpu", func(s *RuntimeSpec) { s.CPU = math.Inf(1) }},
		{"negative memory", func(s *RuntimeSpec) { s.MemoryBytes = -1 }},
		{"relative workdir", func(s *RuntimeSpec) { s.Workdir = "workspace" }},
		{"empty token", func(s *RuntimeSpec) { s.AgentToken = "" }},
		{"unsafe url", func(s *RuntimeSpec) { s.CoderURL = "file:///etc/passwd" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := runtimeTestSpec()
			tc.change(&spec)
			require.Error(t, validateRuntimeSpec(spec))
		})
	}
}

func TestSandboxRuntimePreloadedImageRequired(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing", "not unpacked", "wrong digest"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec := runtimeTestSpec()
			r, client, _ := runtimeTestBackend(t, spec)
			switch name {
			case "missing":
				client.imageError = errdefs.ErrNotFound
			case "not unpacked":
				client.image.unpacked = false
			case "wrong digest":
				client.image.digest = digest.FromString("different image")
			}
			require.Error(t, r.Start(t.Context(), spec))
			entries, err := os.ReadDir(r.networkDirectory)
			require.NoError(t, err)
			require.Empty(t, entries, "failed image validation must not allocate networking")
		})
	}
}

func TestSandboxRuntimePartialStartCleanup(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, network := runtimeTestBackend(t, spec)
	network.setupError = xerrors.New("CNI ADD failed after IPAM allocation")
	require.ErrorContains(t, r.Start(t.Context(), spec), "CNI ADD failed")
	allocations, err := r.List(t.Context(), spec.WorkspaceID)
	require.NoError(t, err)
	require.Equal(t, []RuntimeAllocation{runtimeTestAllocation(spec)}, allocations)
	ownerPath := filepath.Join(r.networkDirectory, spec.ID, "owner.json")
	owner, err := os.ReadFile(ownerPath)
	require.NoError(t, err)
	require.NotContains(t, string(owner), spec.AgentToken)
	r.networkConfig = "changed network configuration"
	require.NoError(t, r.Delete(t.Context(), spec.ID))
	require.Equal(t, 1, network.removes)
	require.Equal(t, "", network.removedPath, "a missing namespace mount uses CNI's reboot recovery path")
	require.NoFileExists(t, ownerPath)
	require.NoError(t, r.Delete(t.Context(), spec.ID), "delete must be idempotent")
}

func TestSandboxRuntimeDeleteRetriesCNI(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, network := runtimeTestBackend(t, spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: runtimeTestAllocation(spec), Config: r.networkConfig}))
	network.removeError = xerrors.New("temporary CNI DEL failure")
	require.ErrorContains(t, r.Delete(t.Context(), spec.ID), "temporary CNI DEL")
	require.FileExists(t, filepath.Join(r.networkDirectory, spec.ID, "owner.json"))
	network.removeError = nil
	require.NoError(t, r.Delete(t.Context(), spec.ID))
	require.Equal(t, 2, network.removes)
}

func TestSandboxRuntimeDeleteRunscOrphan(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, network := runtimeTestBackend(t, spec)
	allocation := runtimeTestAllocation(spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: allocation, Config: r.networkConfig}))
	directory := filepath.Join(r.networkDirectory, spec.ID)
	require.NoError(t, markTaskCreatePending(directory))
	r.removeRunsc = func(_ context.Context, found RuntimeAllocation) (bool, error) {
		require.Equal(t, allocation, found)
		require.Zero(t, network.removes, "network must remain until runtime termination is verified")
		return true, nil
	}
	require.NoError(t, r.Delete(t.Context(), spec.ID), "missing containerd metadata must not hide the owned runtime allocation")
	require.NoDirExists(t, directory)
	require.Equal(t, 1, network.removes)
}

func TestSandboxRuntimeDeletePendingCreateAbsentIsUncertain(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, network := runtimeTestBackend(t, spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: runtimeTestAllocation(spec), Config: r.networkConfig}))
	directory := filepath.Join(r.networkDirectory, spec.ID)
	require.NoError(t, markTaskCreatePending(directory))
	require.ErrorContains(t, r.Delete(t.Context(), spec.ID), "creation outcome remains uncertain")
	require.FileExists(t, filepath.Join(directory, "owner.json"))
	require.Zero(t, network.removes)
}

func TestSandboxRuntimeDeletePendingCreateAfterReboot(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, network := runtimeTestBackend(t, spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: runtimeTestAllocation(spec), Config: r.networkConfig}))
	directory := filepath.Join(r.networkDirectory, spec.ID)
	intent, err := json.Marshal(taskCreateIntent{Version: 1, BootID: uuid.NewString()})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "task-create-pending"), intent, 0o600))
	require.NoError(t, r.Delete(t.Context(), spec.ID), "old-boot intent and verified runtime absence permit cleanup")
	require.NoDirExists(t, directory)
	require.Equal(t, 1, network.removes)
}

func TestSandboxRuntimePendingCreateRejectsInvalidBootIdentity(t *testing.T) {
	t.Parallel()
	for _, data := range []string{`{"version":1,"boot_id":"invalid"}`, `{"version":2,"boot_id":"` + uuid.NewString() + `"}`, `{`, ``} {
		t.Run(data, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "task-create-pending"), []byte(data), 0o600))
			_, err := taskCreatePending(directory)
			require.ErrorIs(t, err, ErrRuntimeOwnership)
		})
	}
}

func TestSandboxRuntimeDeleteRunscFailureRetainsAllocation(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, client, network := runtimeTestBackend(t, spec)
	allocation := runtimeTestAllocation(spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: allocation, Config: r.networkConfig}))
	client.container = runtimeTestContainerFor(allocation)
	r.removeRunsc = func(context.Context, RuntimeAllocation) (bool, error) {
		return false, xerrors.New("runtime deletion could not be verified")
	}
	require.ErrorContains(t, r.Delete(t.Context(), spec.ID), "runtime deletion could not be verified")
	require.False(t, client.container.deleted)
	require.Zero(t, network.removes)
	require.FileExists(t, filepath.Join(r.networkDirectory, spec.ID, "owner.json"))
}

func TestSandboxRuntimeRunscOwnership(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"owned", "absent", "foreign workspace", "missing annotations", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			allocation := runtimeTestAllocation(runtimeTestSpec())
			state := specs.State{ID: allocation.ID, Annotations: allocationLabels(allocation)}
			states := []specs.State{state}
			switch name {
			case "absent":
				states = nil
			case "foreign workspace":
				state.Annotations[workspaceLabel] = uuid.NewString()
			case "missing annotations":
				states[0].Annotations = nil
			case "duplicate":
				states = append(states, state)
			}
			data, err := json.Marshal(states)
			require.NoError(t, err)
			found, err := runscAllocationState(data, allocation)
			switch name {
			case "owned":
				require.NoError(t, err)
				require.NotNil(t, found)
			case "absent":
				require.NoError(t, err)
				require.Nil(t, found)
			default:
				require.ErrorIs(t, err, ErrRuntimeOwnership)
			}
		})
	}
}

func TestSandboxRuntimeRunscCommandCleanup(t *testing.T) {
	t.Parallel()
	allocation := runtimeTestAllocation(runtimeTestSpec())
	binary := filepath.Join(t.TempDir(), "runsc-test")
	// The fixture implements only the scoped inventory and delete commands.
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" >> \"$0.arguments\"\ncase \"$3\" in\nlist) cat \"$0.state\" ;;\ndelete) printf '[]' > \"$0.state\" ;;\n*) exit 2 ;;\nesac\n"
	//nolint:gosec // The test fixture must be executable by the test process.
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700))
	data, err := json.Marshal([]specs.State{{ID: allocation.ID, Annotations: allocationLabels(allocation)}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(binary+".state", data, 0o600))
	removed, err := removeRunscAllocation(t.Context(), binary, allocation)
	require.NoError(t, err)
	require.True(t, removed)
	arguments, err := os.ReadFile(binary + ".arguments")
	require.NoError(t, err)
	rootArgs := "--root=" + sandboxRunscRoot + "\n--platform=systrap\n"
	require.Equal(t, rootArgs+"list\n--format=json\n"+rootArgs+"delete\n--force\n"+allocation.ID+"\n"+rootArgs+"list\n--format=json\n", string(arguments))
}

func TestSandboxRuntimeDeleteIncompleteOwner(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"empty directory", "partial owner", "foreign file", "symlink"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec := runtimeTestSpec()
			r, _, network := runtimeTestBackend(t, spec)
			directory := filepath.Join(r.networkDirectory, spec.ID)
			require.NoError(t, os.Mkdir(directory, 0o700))
			switch name {
			case "partial owner":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "owner.partial"), []byte(`{"alloc`), 0o600))
			case "foreign file":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "unknown"), []byte("retain"), 0o600))
			case "symlink":
				require.NoError(t, os.Symlink(filepath.Join(directory, "unknown"), filepath.Join(directory, "owner.partial")))
			}
			err := r.Delete(t.Context(), spec.ID)
			if name == "foreign file" || name == "symlink" {
				require.Error(t, err)
				require.DirExists(t, directory)
			} else {
				require.NoError(t, err)
				require.NoDirExists(t, directory)
			}
			require.Zero(t, network.removes)
		})
	}
}

func TestSandboxRuntimeEngineRetiresIncompleteIntent(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"empty directory", "partial owner", "unknown file"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			metadata := testMetadata()
			manifest := testManifest(t)
			r, _, network := runtimeTestBackend(t, RuntimeSpec{})
			engine, err := NewEngine(r, t.TempDir())
			require.NoError(t, err)
			intent := State{
				Version: 1, WorkspaceID: metadata.WorkspaceId, BuildID: metadata.WorkspaceBuildId,
				BuildNumber: metadata.WorkspaceBuildNumber, Manifest: manifest, Phase: "intent",
				Transition: metadata.WorkspaceTransition, RuntimeID: "coder-sandbox-" + metadata.WorkspaceBuildId,
				AgentToken: uuid.NewString(),
			}
			require.NoError(t, engine.save(intent))
			directory := filepath.Join(r.networkDirectory, intent.RuntimeID)
			require.NoError(t, os.Mkdir(directory, 0o700))
			if name != "empty directory" {
				require.NoError(t, os.WriteFile(filepath.Join(directory, "owner.partial"), []byte(`{"alloc`), 0o600))
			}
			if name == "unknown file" {
				require.NoError(t, os.WriteFile(filepath.Join(directory, "unknown"), []byte("retain"), 0o600))
			}
			metadata.WorkspaceBuildId = uuid.NewString()
			metadata.WorkspaceBuildNumber++
			metadata.WorkspaceTransition = proto.WorkspaceTransition_STOP
			result, err := engine.Apply(t.Context(), manifest, metadata)
			if name == "unknown file" {
				require.Error(t, err)
				require.FileExists(t, filepath.Join(directory, "unknown"))
			} else {
				require.NoError(t, err)
				require.Equal(t, "stopped", result.Phase)
				require.NoDirExists(t, directory)
			}
			require.Zero(t, network.removes, "network allocation never began")
		})
	}
}

func TestSandboxRuntimeDeleteRejectsForeignResources(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, client, network := runtimeTestBackend(t, spec)
	allocation := runtimeTestAllocation(spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: allocation, Config: r.networkConfig}))
	container := runtimeTestContainerFor(allocation)
	container.info.Labels[workspaceLabel] = uuid.NewString()
	client.container = container
	require.ErrorIs(t, r.Delete(t.Context(), spec.ID), ErrRuntimeOwnership)
	require.False(t, container.deleted)
	require.Zero(t, network.removes)
}

func TestSandboxRuntimeStartConflictDoesNotAuthorizeRollback(t *testing.T) {
	t.Parallel()
	for _, withContainer := range []bool{false, true} {
		t.Run(map[bool]string{false: "network only", true: "running container"}[withContainer], func(t *testing.T) {
			t.Parallel()
			spec := runtimeTestSpec()
			r, client, network := runtimeTestBackend(t, spec)
			allocation := runtimeTestAllocation(spec)
			require.NoError(t, r.writeOwner(networkOwner{Allocation: allocation, Config: r.networkConfig}))
			if withContainer {
				client.container = runtimeTestContainerFor(allocation)
			}
			spec.WorkspaceID = uuid.NewString()
			require.ErrorIs(t, r.Start(t.Context(), spec), ErrRuntimeOwnership)
			require.FileExists(t, filepath.Join(r.networkDirectory, spec.ID, "owner.json"))
			require.Zero(t, network.removes)
		})
	}
}

func TestSandboxRuntimeDeleteRejectsForeignSnapshot(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, client, network := runtimeTestBackend(t, spec)
	snapshotter := &runtimeTestOwnedSnapshotter{info: snapshots.Info{Name: spec.ID, Labels: map[string]string{managedLabel: "false"}}}
	client.snapshotter = snapshotter
	require.ErrorIs(t, r.Delete(t.Context(), spec.ID), ErrRuntimeOwnership)
	require.False(t, snapshotter.removed)
	require.Zero(t, network.removes)
}

func TestSandboxRuntimeDeleteKeepsNetworkWhenTaskCannotStop(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, client, network := runtimeTestBackend(t, spec)
	allocation := runtimeTestAllocation(spec)
	require.NoError(t, r.writeOwner(networkOwner{Allocation: allocation, Config: r.networkConfig}))
	container := runtimeTestContainerFor(allocation)
	container.task = &runtimeTestTask{deleteError: xerrors.New("task is still running")}
	client.container = container
	require.ErrorContains(t, r.Delete(t.Context(), spec.ID), "task is still running")
	require.False(t, container.deleted)
	require.Zero(t, network.removes)
	require.FileExists(t, filepath.Join(r.networkDirectory, spec.ID, "owner.json"))
}

func TestSandboxRuntimeStartsTaskWithoutWaitingForAgent(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, client, _ := runtimeTestBackend(t, spec)
	container := runtimeTestContainerFor(runtimeTestAllocation(spec))
	container.task = &runtimeTestTask{status: containerd.Created}
	client.newContainer = container
	require.NoError(t, r.Start(t.Context(), spec))
	require.True(t, container.task.started)
	client.container = container
	running, err := r.Inspect(t.Context(), spec.ID)
	require.NoError(t, err)
	require.True(t, running)
	require.NoError(t, r.Start(t.Context(), spec), "a matching running allocation is idempotent")
	container.task.status = containerd.Stopped
	require.ErrorContains(t, r.Start(t.Context(), spec), "not running")
}

func TestSandboxRuntimeOwnerRejectsSymlinks(t *testing.T) {
	t.Parallel()
	spec := runtimeTestSpec()
	r, _, _ := runtimeTestBackend(t, spec)
	target := t.TempDir()
	require.NoError(t, os.Symlink(target, filepath.Join(r.networkDirectory, spec.ID)))
	_, err := r.readOwner(spec.ID)
	require.Error(t, err)
}

func TestSandboxResolverRejectsLoopback(t *testing.T) {
	t.Parallel()
	_, err := sandboxResolver(&cni.Result{DNS: []cnitypes.DNS{{Nameservers: []string{"127.0.0.53", "::1", "0.0.0.0"}}}})
	require.Error(t, err)
	value, err := sandboxResolver(&cni.Result{DNS: []cnitypes.DNS{{Nameservers: []string{"1.1.1.1", "1.1.1.1", "2606:4700:4700::1111"}}}})
	require.NoError(t, err)
	require.Equal(t, "nameserver 1.1.1.1\nnameserver 2606:4700:4700::1111\n", value)
}

func runtimeTestBackend(t *testing.T, spec RuntimeSpec) (*containerdRuntime, *runtimeTestClient, *runtimeTestNetwork) {
	t.Helper()
	client := &runtimeTestClient{image: &runtimeTestImage{unpacked: true, digest: digest.Digest("sha256:" + strings.Repeat("a", 64))}}
	network := &runtimeTestNetwork{}
	r := &containerdRuntime{
		client: client, networkDirectory: t.TempDir(), networkConfig: "original configuration",
		removeRunsc: func(context.Context, RuntimeAllocation) (bool, error) { return false, nil },
		newNetwork: func(config string) (cni.CNI, error) {
			require.Equal(t, "original configuration", config)
			return network, nil
		},
		createNamespace: func(path string) error { return os.WriteFile(path, nil, 0o600) },
		removeNamespace: func(path string) error {
			err := os.Remove(path)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		},
	}
	return r, client, network
}

type runtimeTestClient struct {
	image        *runtimeTestImage
	imageError   error
	container    *runtimeTestContainer
	newContainer *runtimeTestContainer
	snapshotter  snapshots.Snapshotter
}

func (c *runtimeTestClient) GetImage(context.Context, string) (containerd.Image, error) {
	return c.image, c.imageError
}
func (c *runtimeTestClient) LoadContainer(context.Context, string) (containerd.Container, error) {
	if c.container == nil || c.container.deleted {
		return nil, errdefs.ErrNotFound
	}
	return c.container, nil
}
func (c *runtimeTestClient) NewContainer(context.Context, string, ...containerd.NewContainerOpts) (containerd.Container, error) {
	if c.newContainer == nil {
		return nil, xerrors.New("unexpected container creation")
	}
	return c.newContainer, nil
}
func (c *runtimeTestClient) Containers(context.Context, ...string) ([]containerd.Container, error) {
	if c.container == nil || c.container.deleted {
		return nil, nil
	}
	return []containerd.Container{c.container}, nil
}
func (c *runtimeTestClient) SnapshotService(string) snapshots.Snapshotter {
	if c.snapshotter != nil {
		return c.snapshotter
	}
	return runtimeTestSnapshotter{}
}
func (*runtimeTestClient) Close() error { return nil }

type runtimeTestImage struct {
	containerd.Image
	unpacked bool
	digest   digest.Digest
}

func (i *runtimeTestImage) Target() imagespec.Descriptor {
	return imagespec.Descriptor{Digest: i.digest}
}
func (i *runtimeTestImage) IsUnpacked(context.Context, string) (bool, error) { return i.unpacked, nil }

type runtimeTestSnapshotter struct{ snapshots.Snapshotter }

func (runtimeTestSnapshotter) Stat(context.Context, string) (snapshots.Info, error) {
	return snapshots.Info{}, errdefs.ErrNotFound
}

type runtimeTestOwnedSnapshotter struct {
	snapshots.Snapshotter
	info    snapshots.Info
	removed bool
}

func (s *runtimeTestOwnedSnapshotter) Stat(context.Context, string) (snapshots.Info, error) {
	if s.removed {
		return snapshots.Info{}, errdefs.ErrNotFound
	}
	return s.info, nil
}

func (s *runtimeTestOwnedSnapshotter) Remove(context.Context, string) error {
	s.removed = true
	return nil
}

type runtimeTestNetwork struct {
	cni.CNI
	setupError, removeError error
	removes                 int
	removedPath             string
}

func (n *runtimeTestNetwork) SetupSerially(context.Context, string, string, ...cni.NamespaceOpts) (*cni.Result, error) {
	return &cni.Result{DNS: []cnitypes.DNS{{Nameservers: []string{"1.1.1.1"}}}}, n.setupError
}
func (n *runtimeTestNetwork) Remove(_ context.Context, _ string, path string, _ ...cni.NamespaceOpts) error {
	n.removes++
	n.removedPath = path
	return n.removeError
}

func runtimeTestContainerFor(allocation RuntimeAllocation) *runtimeTestContainer {
	return &runtimeTestContainer{info: containers.Container{ID: allocation.ID, Image: allocation.Image,
		Runtime: containers.RuntimeInfo{Name: sandboxRuntime}, Snapshotter: sandboxSnapshotter,
		SnapshotKey: allocation.ID, Labels: allocationLabels(allocation)}}
}

type runtimeTestContainer struct {
	containerd.Container
	info    containers.Container
	deleted bool
	task    *runtimeTestTask
}

func (c *runtimeTestContainer) ID() string { return c.info.ID }
func (c *runtimeTestContainer) Info(context.Context, ...containerd.InfoOpts) (containers.Container, error) {
	return c.info, nil
}
func (c *runtimeTestContainer) Delete(context.Context, ...containerd.DeleteOpts) error {
	c.deleted = true
	return nil
}
func (c *runtimeTestContainer) Task(context.Context, cio.Attach) (containerd.Task, error) {
	if c.task == nil {
		return nil, errdefs.ErrNotFound
	}
	return c.task, nil
}
func (c *runtimeTestContainer) NewTask(context.Context, cio.Creator, ...containerd.NewTaskOpts) (containerd.Task, error) {
	return c.task, nil
}

type runtimeTestTask struct {
	containerd.Task
	deleteError error
	started     bool
	status      containerd.ProcessStatus
}

func (t *runtimeTestTask) Start(context.Context) error {
	t.started = true
	t.status = containerd.Running
	return nil
}
func (*runtimeTestTask) Pid() uint32 { return 1234 }
func (t *runtimeTestTask) Status(context.Context) (containerd.Status, error) {
	return containerd.Status{Status: t.status}, nil
}
func (t *runtimeTestTask) Delete(context.Context, ...containerd.ProcessDeleteOpts) (*containerd.ExitStatus, error) {
	return nil, t.deleteError
}
