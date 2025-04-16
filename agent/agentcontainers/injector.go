package agentcontainers

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
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
	i.logger.Info(ctx, "starting injector routine")

	agentScripts := provisionersdk.AgentScriptEnv()
	agentScript := agentScripts[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", runtime.GOOS, runtime.GOARCH)]

	file, err := afero.TempFile(i.fs, "/tmp", "agent-script")
	if err != nil {
		i.logger.Error(ctx, "create agent-script file", slog.Error(err))
		return err
	}
	if _, err := file.Write([]byte(agentScript)); err != nil {
		i.logger.Error(ctx, "write agent-script content", slog.Error(err))
		return err
	}
	if err := file.Close(); err != nil {
		i.logger.Error(ctx, "close agent-script file", slog.Error(err))
		return err
	}

	i.clock.TickerFunc(ctx, 10*time.Second, func() error {
		listing, err := i.cl.List(ctx)
		if err != nil {
			i.logger.Error(ctx, "list containers", slog.Error(err))
			return nil
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
				i.logger.Error(ctx, "create child agent", slog.Error(err))
				return nil
			}

			childAgentID, err := uuid.FromBytes(resp.Id)
			if err != nil {
				i.logger.Error(ctx, "parse agent id", slog.Error(err))
				return nil
			}

			childAuthToken, err := uuid.FromBytes(resp.AuthToken)
			if err != nil {
				i.logger.Error(ctx, "parse auth token", slog.Error(err))
				return nil
			}

			i.children[container.ID] = childAgentID

			accessURL := os.Getenv("CODER_AGENT_URL")
			authType := "token"

			stdout, stderr, err := run(ctx, i.execer,
				"docker", "container", "cp",
				file.Name(),
				fmt.Sprintf("%s:/tmp/bootstrap.sh", container.ID),
			)
			i.logger.Info(ctx, stdout)
			i.logger.Error(ctx, stderr)
			if err != nil {
				i.logger.Error(ctx, "copy bootstrap script", slog.Error(err))
				return nil
			}

			stdout, stderr, err = run(ctx, i.execer, "docker", "container", "exec", container.ID, "chmod +x /tmp/bootstrap.sh")
			i.logger.Info(ctx, stdout)
			i.logger.Error(ctx, stderr)
			if err != nil {
				i.logger.Error(ctx, "make bootstrap script executable", slog.Error(err))
				return nil
			}

			cmd := i.execer.CommandContext(ctx, "docker", "container", "exec", container.ID,
				"--detach",
				"--env", fmt.Sprintf("ACCESS_URL=%s", accessURL),
				"--env", fmt.Sprintf("AUTH_TYPE=%s", authType),
				"--env", fmt.Sprintf("CODER_AGENT_TOKEN=%s", childAuthToken.String()),
				"bash", "-c", "/tmp/bootstrap.sh",
			)

			var stdoutBuf, stderrBuf strings.Builder

			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf

			if err := cmd.Start(); err != nil {
				i.logger.Error(ctx, "starting command", slog.Error(err))
			}

			go func() {
				for {
					i.logger.Info(ctx, stdoutBuf.String())
					i.logger.Error(ctx, stderrBuf.String())

					time.Sleep(5 * time.Second)
				}
			}()

			go func() {
				if err := cmd.Wait(); err != nil {
					i.logger.Error(ctx, "running command", slog.Error(err))
				}
			}()
		}

		return nil
	}, "injector")

	return nil
}
