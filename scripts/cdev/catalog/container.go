package catalog

import (
	"context"
	"io"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// ContainerRunOptions configures a container run.
type ContainerRunOptions struct {
	CreateOpts      docker.CreateContainerOptions
	Logger          slog.Logger
	Detached        bool
	DestroyExisting bool // If true, will remove any existing container with the same name before creating.
	// Stdout is an optional extra writer to tee container stdout
	// into (e.g. a buffer for capturing output). nil = discard.
	Stdout io.Writer
	// Stderr is an optional extra writer to tee container stderr
	// into. nil = discard.
	Stderr io.Writer
}

// ContainerRunResult holds the outcome of a container run.
type ContainerRunResult struct {
	ExitCode  int
	Container *docker.Container
}

// RunContainer creates, attaches to, starts, and waits for a Docker
// container. Container output is streamed through a LogWriter and
// optionally tee'd into opts.Stdout/Stderr. The service name is used
// for labeling log output.
func RunContainer(ctx context.Context, pool *dockertest.Pool, service ServiceName, opts ContainerRunOptions) (*ContainerRunResult, error) {
	logger := opts.Logger.With(slog.F("service", string(service)))

	// Derive a human-readable container name for log lines.
	containerName := opts.CreateOpts.Name
	if containerName == "" {
		return nil, xerrors.New("human container name is required")
	}

	// Always start with the base cdev labels, and include whatever custom labels the
	// caller provided ontop.
	labels := NewLabels()
	for k, v := range opts.CreateOpts.Config.Labels {
		labels[k] = v
	}
	opts.CreateOpts.Config.Labels = labels

	existsFilter := labels.Filter()
	existsFilter["name"] = []string{containerName}
	cnts, err := pool.Client.ListContainers(docker.ListContainersOptions{
		Filters: existsFilter,
	})
	if err != nil {
		return nil, xerrors.Errorf("list containers: %w", err)
	}
	if len(cnts) > 0 {
		if !opts.DestroyExisting {
			return nil, xerrors.Errorf("container with name %q already exists", containerName)
		}
		for _, cnt := range cnts {
			logger.Info(ctx, "removing existing container with same name", slog.F("container_name", containerName))
			if err := pool.Client.RemoveContainer(docker.RemoveContainerOptions{
				ID:    cnt.ID,
				Force: true,
			}); err != nil {
				return nil, xerrors.Errorf("remove existing container: %w", err)
			}
		}
	}

	container, err := pool.Client.CreateContainer(opts.CreateOpts)
	if err != nil {
		return nil, xerrors.Errorf("create container: %w", err)
	}
	defer func() {
		if opts.Detached {
			return // Don't remove if detached since caller is expected to manage lifecycle.
		}
		_ = pool.Client.RemoveContainer(docker.RemoveContainerOptions{
			ID:    container.ID,
			Force: true,
		})
	}()

	// Build output streams with logging.
	stdoutLog := LogWriter(logger, slog.LevelInfo, containerName)
	stderrLog := LogWriter(logger, slog.LevelWarn, containerName)

	var stdoutWriter, stderrWriter io.Writer = stdoutLog, stderrLog
	if opts.Stdout != nil {
		stdoutWriter = io.MultiWriter(stdoutLog, opts.Stdout)
	}
	if opts.Stderr != nil {
		stderrWriter = io.MultiWriter(stderrLog, opts.Stderr)
	}

	// Attach BEFORE starting to capture all output from the beginning.
	attachDone := make(chan error, 1)
	go func() {
		attachDone <- pool.Client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    container.ID,
			OutputStream: stdoutWriter,
			ErrorStream:  stderrWriter,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
		})
	}()

	if err := pool.Client.StartContainer(container.ID, nil); err != nil {
		return nil, xerrors.Errorf("start container: %w", err)
	}

	if opts.Detached {
		// Wait for it to be running at least
		for {
			container, err = pool.Client.InspectContainer(container.ID)
			if err != nil {
				return nil, xerrors.Errorf("inspect container: %w", err)
			}
			if container.State.Running {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}

		// If detached, return immediately without waiting for the container to exit.
		return &ContainerRunResult{
			ExitCode:  0,
			Container: container,
		}, nil
	}
	exitCode, err := pool.Client.WaitContainerWithContext(container.ID, ctx)
	if err != nil {
		return nil, xerrors.Errorf("wait for container: %w", err)
	}

	// Wait for attach to finish (ensures all logs are flushed).
	<-attachDone

	// Close log writers to terminate goroutines.
	_ = stdoutLog.Close()
	_ = stderrLog.Close()

	if exitCode != 0 {
		return nil, xerrors.Errorf("container exited with code %d", exitCode)
	}

	return &ContainerRunResult{
		ExitCode:  exitCode,
		Container: container,
	}, nil
}
