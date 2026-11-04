package agentworkload

import (
	"context"
	"io"
	"maps"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// runtime keeps Docker mechanics out of admission and durable state transitions.
// Errors may contain sensitive daemon responses and must not leave this package.
type workloadRuntime interface {
	Validate(context.Context) (string, error)
	Create(context.Context, string, codersdk.StartHostWorkloadRequest) error
	Attach(context.Context, string) (io.ReadCloser, error)
	Start(context.Context, string) error
	Wait(context.Context, string) (int, error)
	ExecutionDuration(context.Context, string) (time.Duration, error)
	Remove(context.Context, string) error
	Close() error
}

type dockerRuntime struct {
	client  *client.Client
	config  Config
	imageID string
}

func newDockerRuntime(cfg Config) (*dockerRuntime, error) {
	cli, err := client.New(client.WithHost(cfg.DockerHost))
	if err != nil {
		return nil, err
	}
	return &dockerRuntime{client: cli, config: cfg}, nil
}

func (d *dockerRuntime) Validate(ctx context.Context) (string, error) {
	result, err := d.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", xerrors.New("query Docker profile")
	}
	info := result.Info
	if info.OSType != "linux" || info.Driver != "overlay2" || info.CgroupVersion != "2" || !info.MemoryLimit || !info.SwapLimit || !info.CPUCfsQuota || !info.CPUCfsPeriod || !info.PidsLimit {
		return "", xerrors.New("Docker requires Linux overlay2, cgroup v2, CPU quotas, memory/swap and PID limits")
	}
	seccomp := false
	for _, option := range info.SecurityOptions {
		if option == "name=seccomp,profile=builtin" {
			seccomp = true
		}
	}
	if !seccomp {
		return "", xerrors.New("Docker must use the built-in default seccomp profile")
	}
	if d.config.CPU > int64(info.NCPU) || d.config.MemoryBytes > info.MemTotal {
		return "", xerrors.New("configured allocatable capacity exceeds Docker host totals")
	}
	containers, err := d.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return "", xerrors.New("check dedicated Docker daemon")
	}
	if len(containers.Items) != 0 {
		return "", xerrors.New("Docker daemon has surviving containers; reconcile coder-workload-<UUID> containers and use a dedicated empty daemon before enrollment")
	}
	image, err := d.client.ImageInspect(ctx, d.config.Image)
	if err != nil {
		return "", xerrors.New("inspect prepared local image; image preparation must happen before enrollment")
	}
	if image.Config == nil || image.Os != "linux" || len(image.Config.Volumes) != 0 {
		return "", xerrors.New("prepared image must target Linux and declare no VOLUMEs")
	}
	if len(image.ID) != 71 || !strings.HasPrefix(image.ID, "sha256:") || !commitPattern.MatchString(strings.TrimPrefix(image.ID, "sha256:")) {
		return "", xerrors.New("prepared image must resolve to an exact SHA256 image ID")
	}
	d.imageID = image.ID
	return image.ID, nil
}

func (d *dockerRuntime) createOptions(name string, req codersdk.StartHostWorkloadRequest) client.ContainerCreateOptions {
	var env []string
	if _, hasTerm := req.Env["TERM"]; req.TTY && !hasTerm {
		env = []string{"TERM=xterm-256color"}
	}
	for _, key := range slices.Sorted(maps.Keys(req.Env)) {
		env = append(env, key+"="+req.Env[key])
	}
	return client.ContainerCreateOptions{
		Name: name,
		Config: &container.Config{
			Image:       d.imageID,
			Entrypoint:  []string{d.config.Shell, "-c"},
			Cmd:         []string{req.Command},
			WorkingDir:  d.config.Workdir,
			Env:         env,
			Tty:         req.TTY,
			OpenStdin:   req.Stdin,
			AttachStdin: req.Stdin,
			// Pipe EOF needs Docker to close stdin after the primary attach
			// half-closes. WebSocket disconnects never close that attachment.
			StdinOnce:    req.Stdin && !req.TTY,
			AttachStdout: true,
			AttachStderr: true,
			Healthcheck:  &container.HealthConfig{Test: []string{"NONE"}},
			Labels:       map[string]string{"com.coder.workload.id": req.ID.String()},
		},
		HostConfig: &container.HostConfig{
			ConsoleSize:   [2]uint{uint(req.Height), uint(req.Width)},
			NetworkMode:   "bridge",
			CapDrop:       []string{"ALL"},
			SecurityOpt:   []string{"no-new-privileges:true"},
			CgroupnsMode:  "private",
			IpcMode:       "private",
			RestartPolicy: container.RestartPolicy{Name: "no"},
			LogConfig:     container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "2m", "max-file": "2"}},
			Resources: container.Resources{
				CPUPeriod:  100000,
				CPUQuota:   req.CPU * 100000,
				Memory:     req.MemoryBytes,
				MemorySwap: req.MemoryBytes,
				PidsLimit:  new(int64(256)),
			},
		},
	}
}

func (d *dockerRuntime) Create(ctx context.Context, name string, req codersdk.StartHostWorkloadRequest) error {
	_, err := d.client.ContainerCreate(ctx, d.createOptions(name, req))
	return err
}

type dockerAttachment struct{ client.ContainerAttachResult }

func (a dockerAttachment) Read(p []byte) (int, error) { return a.Reader.Read(p) }
func (a dockerAttachment) Close() error               { a.ContainerAttachResult.Close(); return nil }
func (a dockerAttachment) CloseWrite() error          { return a.ContainerAttachResult.CloseWrite() }
func (a dockerAttachment) Write(p []byte) (int, error) {
	return a.WriteContext(context.Background(), p)
}

// WriteContext bounds a blocked Docker write without closing its output stream.
// The single input controller serializes writes and joins cancellation before
// releasing control, so a detached controller cannot poison its successor.
func (a dockerAttachment) WriteContext(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := a.Conn.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil {
		return 0, err
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = a.Conn.SetWriteDeadline(time.Now())
		close(done)
	})
	defer func() {
		if !stop() {
			<-done
		}
		_ = a.Conn.SetWriteDeadline(time.Time{})
	}()
	return a.Conn.Write(p)
}

func (d *dockerRuntime) Attach(ctx context.Context, name string) (io.ReadCloser, error) {
	return d.attach(ctx, name, false)
}

func (d *dockerRuntime) AttachInput(ctx context.Context, name string) (io.ReadCloser, error) {
	return d.attach(ctx, name, true)
}

func (d *dockerRuntime) Resize(ctx context.Context, name string, width, height uint16) error {
	_, err := d.client.ContainerResize(ctx, name, client.ContainerResizeOptions{Width: uint(width), Height: uint(height)})
	return err
}

func (d *dockerRuntime) attach(ctx context.Context, name string, stdin bool) (io.ReadCloser, error) {
	// The SDK's hijack handshake reads the socket directly, without observing
	// request cancellation. Bound that handshake, then detach the connection
	// from its setup context so streaming can outlive the operation timeout.
	stopCancel := func() bool { return true }
	cli, err := client.New(client.WithHost(d.config.DockerHost), client.WithAPIVersion(d.client.ClientVersion()), client.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) {
		conn, err := d.client.Dialer()(ctx)
		if err != nil {
			return nil, err
		}
		stopCancel = context.AfterFunc(ctx, func() { _ = conn.Close() })
		return conn, nil
	}))
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	res, err := cli.ContainerAttach(ctx, name, client.ContainerAttachOptions{Stream: true, Stdin: stdin, Stdout: true, Stderr: true})
	stopCancel()
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		res.Close()
		return nil, ctx.Err()
	}
	return dockerAttachment{res}, nil
}

func (d *dockerRuntime) Start(ctx context.Context, name string) error {
	_, err := d.client.ContainerStart(ctx, name, client.ContainerStartOptions{})
	return err
}

func (d *dockerRuntime) Wait(ctx context.Context, name string) (int, error) {
	wait := d.client.ContainerWait(ctx, name, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	// Always consume the SDK result. Selecting ctx.Done here could abandon
	// its unbuffered result sender when exit races cancellation. The HTTP
	// request itself observes ctx and delivers cancellation on wait.Error.
	select {
	case err := <-wait.Error:
		return 0, err
	case result := <-wait.Result:
		if result.Error != nil {
			return 0, xerrors.New("Docker wait failed")
		}
		return int(result.StatusCode), nil
	}
}

func (d *dockerRuntime) ExecutionDuration(ctx context.Context, name string) (time.Duration, error) {
	result, err := d.client.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return 0, xerrors.Errorf("inspect container execution duration: %w", err)
	}
	state := result.Container.State
	if state == nil {
		return 0, xerrors.New("container execution timestamps unavailable")
	}
	started, err := time.Parse(time.RFC3339Nano, state.StartedAt)
	if err != nil || started.IsZero() {
		return 0, xerrors.New("container start timestamp unavailable")
	}
	finished, err := time.Parse(time.RFC3339Nano, state.FinishedAt)
	if err != nil || finished.IsZero() || finished.Before(started) {
		return 0, xerrors.New("container finish timestamp unavailable")
	}
	return finished.Sub(started), nil
}

func (d *dockerRuntime) Remove(ctx context.Context, name string) error {
	_, err := d.client.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	// Even NotFound is ambiguous after an uncertain create. Do not reinterpret
	// errors as proof that an outstanding reservation can be released.
	return err
}
func (d *dockerRuntime) Close() error { return d.client.Close() }
