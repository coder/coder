package agentcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/quartz"
	"github.com/google/uuid"
	"github.com/spf13/afero"
)

type Injector struct {
	fs     afero.Fs
	cl     Lister
	logger slog.Logger
	clock  quartz.Clock
	api    proto.DRPCAgentClient25
	execer agentexec.Execer

	children map[string]uuid.UUID
}

func NewInjector(
	fs afero.Fs,
	logger slog.Logger,
	clock quartz.Clock,
	cl Lister,
	api proto.DRPCAgentClient25,
	execer agentexec.Execer,
) *Injector {
	return &Injector{
		fs:       fs,
		cl:       cl,
		api:      api,
		logger:   logger,
		clock:    clock,
		execer:   execer,
		children: make(map[string]uuid.UUID),
	}
}

func (i *Injector) Start(ctx context.Context) error {
	agentScripts := provisionersdk.AgentScriptEnv()
	agentScript := agentScripts[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", runtime.GOOS, runtime.GOARCH)]

	file, err := afero.TempFile(i.fs, "/tmp", "agentScript")
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte(agentScript)); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	i.clock.TickerFunc(ctx, 10*time.Second, func() error {
		listing, err := i.cl.List(ctx)
		if err != nil {
			return err
		}

		for _, container := range listing.Containers {
			workspaceFolder, exists := container.Labels[DevcontainerLocalFolderLabel]
			if !exists || workspaceFolder == "" {
				continue
			}

			// Child has already been injected with the agent, we can ignore it.
			if _, childInjected := i.children[container.ID]; childInjected {
				continue
			}

			resp, err := i.api.CreateChildAgent(ctx, &proto.CreateChildAgentRequest{
				Name:      container.FriendlyName,
				Directory: workspaceFolder,
			})
			if err != nil {
				return err
			}

			childAgentID, err := uuid.FromBytes(resp.Id)
			if err != nil {
				return err
			}

			i.children[container.ID] = childAgentID

			accessURL := os.Getenv("CODER_AGENT_URL")
			authType := "token"

			stdout, stderr, err := run(ctx, i.execer,
				"docker", "container", "cp",
				filepath.Join("/tmp", file.Name()),
				fmt.Sprintf("%s:/tmp/bootstrap.sh", container.ID),
			)
			if err != nil {
				return err
			}

			i.logger.Info(ctx, stdout)
			i.logger.Error(ctx, stderr)

			stdout, stderr, err = run(ctx, i.execer, "docker", "container", "exec", container.ID, "chmod +x /tmp/bootstrap.sh")
			if err != nil {
				return err
			}

			i.logger.Info(ctx, stdout)
			i.logger.Error(ctx, stderr)

			stdout, stderr, err = run(ctx, i.execer, "docker", "container", "exec", container.ID,
				"-e", fmt.Sprintf("ACCESS_URL=%s", accessURL),
				"-e", fmt.Sprintf("AUTH_TYPE=%s", authType),
				"/tmp/bootstrap.sh",
			)
			if err != nil {
				return err
			}

			i.logger.Info(ctx, stdout)
			i.logger.Error(ctx, stderr)
		}

		return nil
	}, "injector")

	return nil
}
