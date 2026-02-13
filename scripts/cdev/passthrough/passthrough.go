package passthrough

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/scripts/cdev/workingdir"
)

func DockerComposePassthroughCmd(ctx context.Context) error {
	logger := slog.Make(sloghuman.Sink(os.Stderr))
	working := workingdir.From(ctx)

	logger.Info(ctx, "docker-compose passthrough", slog.F("dir", working.Root()))
	groupID := 999
	if gid, err := dockerGroupID(); err != nil {
		logger.Warn(ctx, "failed to get docker group ID, using default", slog.Error(err), slog.F("default_gid", groupID))
	} else {
		groupID = gid
	}

	args := append([]string{"compose", "-f", "docker-compose.dev.yml"}, os.Args[1:]...)
	primaryCmd := os.Args[1]
	switch primaryCmd {
	case "up":
		args = append(args, "--force-recreate", "cdevsetup", "--build", "cdevsetup")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	logger.Info(ctx, "running docker-compose command", slog.F("cmd", cmd.String()))
	cmd.Env = append(cmd.Env, "DOCKER_GROUP="+strconv.Itoa(groupID))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = working.Root()
	return cmd.Run()

}

// dockerGroupID returns the GID of the docker group by running `getent group docker`.
// The output format is `docker:x:GID:users`, e.g., `docker:x:131:steven`.
func dockerGroupID() (int, error) {
	cmd := exec.Command("getent", "group", "docker")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("getent group docker: %w", err)
	}
	// Format: docker:x:131:steven
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) < 3 {
		return 0, fmt.Errorf("unexpected getent output: %q", string(out))
	}
	gid, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("parse gid %q: %w", parts[2], err)
	}
	return gid, nil
}
