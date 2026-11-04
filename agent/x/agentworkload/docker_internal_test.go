//go:build linux

package agentworkload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func validDockerInfo() system.Info {
	return system.Info{OSType: "linux", Driver: "overlay2", CgroupVersion: "2", MemoryLimit: true, SwapLimit: true, CPUCfsQuota: true, CPUCfsPeriod: true, PidsLimit: true, NCPU: 8, MemTotal: 8 << 30, SecurityOptions: []string{"name=seccomp,profile=builtin"}}
}

//nolint:revive // Flags select invalid Docker profiles for validation tests.
func dockerTestRuntime(t *testing.T, cfg Config, info system.Info, volumes bool, survivors bool, create chan<- container.CreateRequest) *dockerRuntime {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.54/info":
			_ = json.NewEncoder(w).Encode(info)
		case r.URL.Path == "/v1.54/containers/json":
			if survivors {
				_ = json.NewEncoder(w).Encode([]container.Summary{{ID: "surviving"}})
			} else {
				_, _ = w.Write([]byte("[]"))
			}
		case strings.HasPrefix(r.URL.Path, "/v1.54/images/"):
			img := image.InspectResponse{ID: testImageID, Os: "linux", Config: &dockerspec.DockerOCIImageConfig{}}
			if volumes {
				img.Config.Volumes = map[string]struct{}{"/repo": {}}
			}
			_ = json.NewEncoder(w).Encode(img)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.54/containers/create":
			var req container.CreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			create <- req
			_ = json.NewEncoder(w).Encode(container.CreateResponse{ID: "created"})
		default:
			http.Error(w, "unexpected Docker request", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.54"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	return &dockerRuntime{client: cli, config: cfg}
}

func TestDockerProfileAndCreate(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	require.NoError(t, cfg.validate())
	created := make(chan container.CreateRequest, 1)
	d := dockerTestRuntime(t, cfg, validDockerInfo(), false, false, created)
	id, err := d.Validate(t.Context())
	require.NoError(t, err)
	require.Equal(t, testImageID, id)
	req := testRequest()
	req.Env = map[string]string{"Z_VALUE": "a=b,c\nsecond line\n", "EMPTY": "", "_KEY": "雪 'quoted' $HOME"}
	require.NoError(t, d.Create(t.Context(), containerName(req.ID), req))
	actual := <-created
	require.Equal(t, testImageID, actual.Image)
	require.Equal(t, []string{"/bin/sh", "-c"}, actual.Entrypoint)
	require.Equal(t, []string{req.Command}, actual.Cmd)
	require.Equal(t, cfg.Workdir, actual.WorkingDir)
	require.Equal(t, []string{"EMPTY=", "Z_VALUE=a=b,c\nsecond line\n", "_KEY=雪 'quoted' $HOME"}, actual.Env)
	require.Empty(t, actual.Volumes)
	require.Equal(t, []string{"NONE"}, actual.Healthcheck.Test)
	require.False(t, actual.Tty)
	require.False(t, actual.OpenStdin)
	host := actual.HostConfig
	require.False(t, actual.NetworkDisabled)
	require.Equal(t, container.NetworkMode("bridge"), host.NetworkMode)
	require.Equal(t, []string{"ALL"}, host.CapDrop)
	require.Empty(t, host.CapAdd)
	require.Equal(t, []string{"no-new-privileges:true"}, host.SecurityOpt)
	require.Empty(t, host.Binds)
	require.Empty(t, host.Mounts)
	require.Empty(t, host.VolumesFrom)
	require.Empty(t, host.Devices)
	require.Empty(t, host.CpusetCpus)
	require.False(t, host.Privileged)
	require.False(t, host.ReadonlyRootfs)
	require.False(t, host.AutoRemove)
	require.Equal(t, int64(256), *host.PidsLimit)
	require.Equal(t, req.CPU*host.CPUPeriod, host.CPUQuota)
	require.Equal(t, req.MemoryBytes, host.Memory)
	require.Equal(t, req.MemoryBytes, host.MemorySwap)
	require.Equal(t, "json-file", host.LogConfig.Type)
	require.Equal(t, map[string]string{"max-size": "2m", "max-file": "2"}, host.LogConfig.Config)
}

func TestDockerInteractiveCreateOptions(t *testing.T) {
	t.Parallel()
	for _, tty := range []bool{false, true} {
		t.Run(map[bool]string{false: "Pipe", true: "TTY"}[tty], func(t *testing.T) {
			t.Parallel()
			d := &dockerRuntime{config: testConfig(t), imageID: testImageID}
			req := testRequest()
			req.Stdin, req.TTY = true, tty
			if tty {
				req.Width, req.Height = 132, 43
			}
			options := d.createOptions(containerName(req.ID), req)
			require.True(t, options.Config.OpenStdin)
			require.True(t, options.Config.AttachStdin)
			require.Equal(t, tty, options.Config.Tty)
			require.Equal(t, !tty, options.Config.StdinOnce)
			if tty {
				require.Equal(t, []string{"TERM=xterm-256color"}, options.Config.Env)
				require.Equal(t, [2]uint{43, 132}, options.HostConfig.ConsoleSize)
			} else {
				require.Empty(t, options.Config.Env)
				require.Equal(t, [2]uint{}, options.HostConfig.ConsoleSize)
			}
		})
	}
}

func TestDockerEnvCreateOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		tty  bool
		env  map[string]string
		want []string
	}{
		{name: "TTYDefault", tty: true, env: map[string]string{"A": ""}, want: []string{"TERM=xterm-256color", "A="}},
		{name: "TTYOverride", tty: true, env: map[string]string{"TERM": "vt100", "A": "a=b,c\n"}, want: []string{"A=a=b,c\n", "TERM=vt100"}},
		{name: "TTYEmptyOverride", tty: true, env: map[string]string{"TERM": ""}, want: []string{"TERM="}},
		{name: "PipeOverride", env: map[string]string{"TERM": "dumb"}, want: []string{"TERM=dumb"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := &dockerRuntime{config: testConfig(t), imageID: testImageID}
			req := testRequest()
			req.Env, req.TTY = tc.env, tc.tty
			options := d.createOptions(containerName(req.ID), req)
			require.Equal(t, tc.want, options.Config.Env)
		})
	}
}

func TestDockerExecutionDuration(t *testing.T) {
	t.Parallel()
	const started = "2026-11-04T12:00:00.123456789Z"
	for _, tc := range []struct {
		name   string
		state  *container.State
		status int
		want   time.Duration
	}{
		{name: "Nanoseconds", state: &container.State{StartedAt: started, FinishedAt: "2026-11-04T12:00:02.987654321Z"}, want: 2*time.Second + 864197532*time.Nanosecond},
		{name: "Timezones", state: &container.State{StartedAt: started, FinishedAt: "2026-11-04T13:00:01.123456789+01:00"}, want: time.Second},
		{name: "ZeroDuration", state: &container.State{StartedAt: started, FinishedAt: started}},
		{name: "InspectError", status: http.StatusInternalServerError},
		{name: "MissingState"},
		{name: "MissingStart", state: &container.State{FinishedAt: started}},
		{name: "MissingFinish", state: &container.State{StartedAt: started}},
		{name: "InvalidStart", state: &container.State{StartedAt: "invalid", FinishedAt: started}},
		{name: "InvalidFinish", state: &container.State{StartedAt: started, FinishedAt: "invalid"}},
		{name: "ZeroStart", state: &container.State{StartedAt: "0001-01-01T00:00:00Z", FinishedAt: started}},
		{name: "ZeroFinish", state: &container.State{StartedAt: started, FinishedAt: "0001-01-01T00:00:00Z"}},
		{name: "FinishBeforeStart", state: &container.State{StartedAt: started, FinishedAt: "2026-11-04T11:59:59Z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name := containerName(testRequest().ID)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1.54/containers/"+name+"/json" {
					t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusBadRequest)
					return
				}
				if tc.status != 0 {
					http.Error(w, "inspect failed", tc.status)
					return
				}
				_ = json.NewEncoder(w).Encode(container.InspectResponse{State: tc.state})
			}))
			t.Cleanup(server.Close)
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.54"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cli.Close()) })
			d := &dockerRuntime{client: cli}
			duration, err := d.ExecutionDuration(t.Context(), name)
			if tc.want == 0 && tc.name != "ZeroDuration" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, duration)
		})
	}
}

func TestRejectDockerProfiles(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{"os", "driver", "cgroup", "memory", "swap", "quota", "pids", "seccomp", "capacity", "volumes", "survivors"} {
		t.Run(invalid, func(t *testing.T) {
			t.Parallel()
			info := validDockerInfo()
			switch invalid {
			case "os":
				info.OSType = "windows"
			case "driver":
				info.Driver = "vfs"
			case "cgroup":
				info.CgroupVersion = "1"
			case "memory":
				info.MemoryLimit = false
			case "swap":
				info.SwapLimit = false
			case "quota":
				info.CPUCfsQuota = false
			case "pids":
				info.PidsLimit = false
			case "seccomp":
				info.SecurityOptions = nil
			case "capacity":
				info.NCPU = 1
			}
			d := dockerTestRuntime(t, testConfig(t), info, invalid == "volumes", invalid == "survivors", nil)
			_, err := d.Validate(t.Context())
			require.Error(t, err)
		})
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	require.NoError(t, cfg.validate())
	require.Equal(t, "/bin/sh", cfg.Shell)
	require.Equal(t, "unix:///var/run/docker.sock", cfg.DockerHost)
	for _, invalid := range []string{"token", "workdir", "shell", "cpu", "memory", "repositories", "docker_host"} {
		t.Run(invalid, func(t *testing.T) {
			t.Parallel()
			bad := cfg
			switch invalid {
			case "token":
				bad.Token = "short"
			case "workdir":
				bad.Workdir = "relative"
			case "shell":
				bad.Shell = "sh"
			case "cpu":
				bad.CPU = 0
			case "memory":
				bad.MemoryBytes = 1
			case "repositories":
				bad.Repositories = map[string]string{"repo": "branch-name"}
			case "docker_host":
				bad.DockerHost = "tcp://remote:2375"
			}
			require.Error(t, bad.validate())
		})
	}
	file := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	//nolint:gosec // Verify that enrollment rejects a world-readable config.
	require.NoError(t, os.WriteFile(file, data, 0o644))
	_, err = NewFromFile(t.Context(), file)
	require.ErrorContains(t, err, "private workload config")
	require.NoError(t, os.Chmod(file, 0o600))
	link := file + ".link"
	require.NoError(t, os.Symlink(file, link))
	_, err = NewFromFile(t.Context(), link)
	require.ErrorContains(t, err, "private workload config")
	cfg.Token = "short"
	data, err = json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0o600))
	_, err = NewFromFile(t.Context(), file)
	require.ErrorContains(t, err, "token must contain")
}

// This opt-in test only uses an already prepared image and an empty dedicated
// daemon. It does not build, pull, or modify the user's daemon configuration.
func TestDockerIntegration(t *testing.T) {
	t.Parallel()
	imageName := os.Getenv("CODER_WORKLOAD_TEST_IMAGE")
	if imageName == "" {
		t.Skip("set CODER_WORKLOAD_TEST_IMAGE and optionally CODER_WORKLOAD_TEST_DOCKER_HOST for a dedicated empty Docker daemon")
	}
	cfg := testConfig(t)
	cfg.Image = imageName
	cfg.CPU = 1
	cfg.Workdir = "/"
	cfg.DockerHost = os.Getenv("CODER_WORKLOAD_TEST_DOCKER_HOST")
	require.NoError(t, cfg.validate())
	file := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	s, err := NewFromFile(ctx, file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	req := testRequest()
	req.GenerationID = s.generation.ID
	req.Command = "printf stdout; printf stderr >&2"
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	joinRuns(t, s)
	out := getOutput(t, s, req.ID, "0")
	require.Equal(t, "succeeded", out.Workload.State)
	require.False(t, out.Workload.CleanupPending)
	streams := map[string]string{}
	for _, event := range out.Events {
		streams[event.Stream] += string(event.Data)
	}
	require.Equal(t, map[string]string{"stdout": "stdout", "stderr": "stderr"}, streams)
	var info codersdk.WorkloadHostInfo
	require.NoError(t, json.Unmarshal(requestHTTP(t, s, http.MethodGet, "/info", nil).Body.Bytes(), &info))
	require.Equal(t, cfg.CPU, info.AvailableCPU)
}
