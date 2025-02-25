package agentcontainers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/user"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk"

	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"
)

const (
	RuntimeSystem int = iota
	RuntimeDocker
)

// DockerCLILister is a ContainerLister that lists containers using the docker CLI
type DockerCLILister struct {
	execer agentexec.Execer
}

var _ Lister = &DockerCLILister{}

func NewDocker(execer agentexec.Execer) Lister {
	return &DockerCLILister{
		execer: agentexec.DefaultExecer,
	}
}

// DockerEnvInfoer is an implementation of agentssh.EnvInfoer that returns
// information about a container.
type DockerEnvInfoer struct {
	usershell.SystemEnvInfo
	container string
	user      *user.User
	userShell string
	env       []string
}

// EnvInfo returns information about the environment of a container.
func EnvInfo(ctx context.Context, execer agentexec.Execer, container, containerUser string) (*DockerEnvInfoer, error) {
	var dei DockerEnvInfoer
	dei.container = container

	if containerUser == "" {
		// Get the "default" user of the container if no user is specified.
		// TODO: handle different container runtimes.
		cmd, args := wrapDockerExec(container, "", "whoami")
		stdout, stderr, err := run(ctx, execer, cmd, args...)
		if err != nil {
			return nil, xerrors.Errorf("get container user: run whoami: %w: %s", err, stderr)
		}
		if len(stdout) == 0 {
			return nil, xerrors.Errorf("get container user: run whoami: empty output")
		}
		containerUser = stdout
	}
	// Now that we know the username, get the required info from the container.
	// We can't assume the presence of `getent` so we'll just have to sniff /etc/passwd.
	cmd, args := wrapDockerExec(container, containerUser, "cat", "/etc/passwd")
	stdout, stderr, err := run(ctx, execer, cmd, args...)
	if err != nil {
		return nil, xerrors.Errorf("get container user: read /etc/passwd: %w: %q", err, stderr)
	}

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	var foundLine string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, containerUser+":") {
			continue
		}
		foundLine = line
		break
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("get container user: scan /etc/passwd: %w", err)
	}
	if foundLine == "" {
		return nil, xerrors.Errorf("get container user: no matching entry for %q found in /etc/passwd", containerUser)
	}

	// Parse the output of /etc/passwd. It looks like this:
	// postgres:x:999:999::/var/lib/postgresql:/bin/bash
	passwdFields := strings.Split(foundLine, ":")
	if len(passwdFields) != 7 {
		return nil, xerrors.Errorf("get container user: invalid line in /etc/passwd: %q", foundLine)
	}

	// The fifth entry in /etc/passwd contains GECOS information, which is a
	// comma-separated list of fields. The first field is the user's full name.
	gecos := strings.Split(passwdFields[4], ",")
	fullName := ""
	if len(gecos) > 1 {
		fullName = gecos[0]
	}

	dei.user = &user.User{
		Gid:      passwdFields[3],
		HomeDir:  passwdFields[5],
		Name:     fullName,
		Uid:      passwdFields[2],
		Username: containerUser,
	}
	dei.userShell = passwdFields[6]

	// We need to inspect the container labels for remoteEnv and append these to
	// the resulting docker exec command.
	// ref: https://code.visualstudio.com/docs/devcontainers/attach-container
	env, err := devcontainerEnv(ctx, execer, container)
	if err != nil { // best effort.
		return nil, xerrors.Errorf("read devcontainer remoteEnv: %w", err)
	}
	dei.env = env

	return &dei, nil
}

func (dei *DockerEnvInfoer) User() (*user.User, error) {
	// Clone the user so that the caller can't modify it
	u := *dei.user
	return &u, nil
}

func (dei *DockerEnvInfoer) Shell(string) (string, error) {
	return dei.userShell, nil
}

func (dei *DockerEnvInfoer) ModifyCommand(cmd string, args ...string) (string, []string) {
	// Wrap the command with `docker exec` and run it as the container user.
	// There is some additional munging here regarding the container user and environment.
	dockerArgs := []string{
		"exec",
		// The assumption is that this command will be a shell command, so allocate a PTY.
		"--interactive",
		"--tty",
		// Run the command as the user in the container.
		"--user",
		dei.user.Username,
		// Set the working directory to the user's home directory as a sane default.
		"--workdir",
		dei.user.HomeDir,
	}

	// Append the environment variables from the container.
	for _, e := range dei.env {
		dockerArgs = append(dockerArgs, "--env", e)
	}

	// Append the container name and the command.
	dockerArgs = append(dockerArgs, dei.container, cmd)
	return "docker", append(dockerArgs, args...)
}

// devcontainerEnv is a helper function that inspects the container labels to
// find the required environment variables for running a command in the container.
func devcontainerEnv(ctx context.Context, execer agentexec.Execer, container string) ([]string, error) {
	ins, stderr, err := runDockerInspect(ctx, execer, container)
	if err != nil {
		return nil, xerrors.Errorf("inspect container: %w: %q", err, stderr)
	}

	if len(ins) != 1 {
		return nil, xerrors.Errorf("inspect container: expected 1 container, got %d", len(ins))
	}

	in := ins[0]
	if in.Config.Labels == nil {
		return nil, nil
	}

	// We want to look for the devcontainer metadata, which is in the
	// value of the label `devcontainer.metadata`.
	rawMeta, ok := in.Config.Labels["devcontainer.metadata"]
	if !ok {
		return nil, nil
	}
	meta := struct {
		RemoteEnv map[string]string `json:"remoteEnv"`
	}{}
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		return nil, xerrors.Errorf("unmarshal devcontainer.metadata: %w", err)
	}

	// The environment variables are stored in the `remoteEnv` key.
	env := make([]string, 0, len(meta.RemoteEnv))
	for k, v := range meta.RemoteEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	slices.Sort(env)
	return env, nil
}

// wrapDockerExec is a helper function that wraps the given command and arguments
// with a docker exec command that runs as the given user in the given
// container. This is used to fetch information about a container prior to
// running the actual command.
func wrapDockerExec(containerName, userName, cmd string, args ...string) (string, []string) {
	dockerArgs := []string{"exec", "--interactive"}
	if userName != "" {
		dockerArgs = append(dockerArgs, "--user", userName)
	}
	dockerArgs = append(dockerArgs, containerName, cmd)
	return "docker", append(dockerArgs, args...)
}

// Helper function to run a command and return its stdout and stderr.
// We want to differentiate stdout and stderr instead of using CombinedOutput.
// We also want to differentiate between a command running successfully with
// output to stderr and a non-zero exit code.
func run(ctx context.Context, execer agentexec.Execer, cmd string, args ...string) (stdout, stderr string, err error) {
	var stdoutBuf, stderrBuf strings.Builder
	execCmd := execer.CommandContext(ctx, cmd, args...)
	execCmd.Stdout = &stdoutBuf
	execCmd.Stderr = &stderrBuf
	err = execCmd.Run()
	stdout = strings.TrimSpace(stdoutBuf.String())
	stderr = strings.TrimSpace(stderrBuf.String())
	return stdout, stderr, err
}

func (dcl *DockerCLILister) List(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	// List all container IDs, one per line, with no truncation
	cmd := dcl.execer.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--no-trunc")
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		// TODO(Cian): detect specific errors:
		// - docker not installed
		// - docker not running
		// - no permissions to talk to docker
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("run docker ps: %w: %q", err, strings.TrimSpace(stderrBuf.String()))
	}

	ids := make([]string, 0)
	scanner := bufio.NewScanner(&stdoutBuf)
	for scanner.Scan() {
		tmp := strings.TrimSpace(scanner.Text())
		if tmp == "" {
			continue
		}
		ids = append(ids, tmp)
	}
	if err := scanner.Err(); err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("scan docker ps output: %w", err)
	}

	dockerPsStderr := strings.TrimSpace(stderrBuf.String())
	if len(ids) == 0 {
		return codersdk.WorkspaceAgentListContainersResponse{
			Warnings: []string{dockerPsStderr},
		}, nil
	}

	// now we can get the detailed information for each container
	// Run `docker inspect` on each container ID.
	// NOTE: There is an unavoidable potential race condition where a
	// container is removed between `docker ps` and `docker inspect`.
	// In this case, stderr will contain an error message but stdout
	// will still contain valid JSON. We will just end up missing
	// information about the removed container. We could potentially
	// log this error, but I'm not sure it's worth it.
	ins, dockerInspectStderr, err := runDockerInspect(ctx, dcl.execer, ids...)
	if err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("run docker inspect: %w", err)
	}

	res := codersdk.WorkspaceAgentListContainersResponse{
		Containers: make([]codersdk.WorkspaceAgentDevcontainer, len(ins)),
	}
	for idx, in := range ins {
		out, warns := convertDockerInspect(in)
		res.Warnings = append(res.Warnings, warns...)
		res.Containers[idx] = out
	}

	if dockerPsStderr != "" {
		res.Warnings = append(res.Warnings, dockerPsStderr)
	}
	if dockerInspectStderr != "" {
		res.Warnings = append(res.Warnings, dockerInspectStderr)
	}

	return res, nil
}

// runDockerInspect is a helper function that runs `docker inspect` on the given
// container IDs and returns the parsed output.
// The stderr output is also returned for logging purposes.
func runDockerInspect(ctx context.Context, execer agentexec.Execer, ids ...string) ([]dockerInspect, string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := execer.CommandContext(ctx, "docker", append([]string{"inspect"}, ids...)...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	stderr := strings.TrimSpace(stderrBuf.String())
	if err != nil {
		return nil, stderr, err
	}

	var ins []dockerInspect
	if err := json.NewDecoder(&stdoutBuf).Decode(&ins); err != nil {
		return nil, stderr, xerrors.Errorf("decode docker inspect output: %w", err)
	}

	return ins, stderr, nil
}

// To avoid a direct dependency on the Docker API, we use the docker CLI
// to fetch information about containers.
type dockerInspect struct {
	ID         string                  `json:"Id"`
	Created    time.Time               `json:"Created"`
	Config     dockerInspectConfig     `json:"Config"`
	HostConfig dockerInspectHostConfig `json:"HostConfig"`
	Name       string                  `json:"Name"`
	Mounts     []dockerInspectMount    `json:"Mounts"`
	State      dockerInspectState      `json:"State"`
}

type dockerInspectConfig struct {
	Image  string            `json:"Image"`
	Labels map[string]string `json:"Labels"`
}

type dockerInspectHostConfig struct {
	PortBindings map[string]any `json:"PortBindings"`
}

type dockerInspectMount struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Type        string `json:"Type"`
}

type dockerInspectState struct {
	Running  bool   `json:"Running"`
	ExitCode int    `json:"ExitCode"`
	Error    string `json:"Error"`
}

func (dis dockerInspectState) String() string {
	if dis.Running {
		return "running"
	}
	var sb strings.Builder
	_, _ = sb.WriteString("exited")
	if dis.ExitCode != 0 {
		_, _ = sb.WriteString(fmt.Sprintf(" with code %d", dis.ExitCode))
	} else {
		_, _ = sb.WriteString(" successfully")
	}
	if dis.Error != "" {
		_, _ = sb.WriteString(fmt.Sprintf(": %s", dis.Error))
	}
	return sb.String()
}

func convertDockerInspect(in dockerInspect) (codersdk.WorkspaceAgentDevcontainer, []string) {
	var warns []string
	out := codersdk.WorkspaceAgentDevcontainer{
		CreatedAt: in.Created,
		// Remove the leading slash from the container name
		FriendlyName: strings.TrimPrefix(in.Name, "/"),
		ID:           in.ID,
		Image:        in.Config.Image,
		Labels:       in.Config.Labels,
		Ports:        make([]codersdk.WorkspaceAgentListeningPort, 0),
		Running:      in.State.Running,
		Status:       in.State.String(),
		Volumes:      make(map[string]string, len(in.Mounts)),
	}

	if in.HostConfig.PortBindings == nil {
		in.HostConfig.PortBindings = make(map[string]any)
	}
	portKeys := maps.Keys(in.HostConfig.PortBindings)
	// Sort the ports for deterministic output.
	sort.Strings(portKeys)
	for _, p := range portKeys {
		if port, network, err := convertDockerPort(p); err != nil {
			warns = append(warns, err.Error())
		} else {
			out.Ports = append(out.Ports, codersdk.WorkspaceAgentListeningPort{
				Network: network,
				Port:    port,
			})
		}
	}

	if in.Mounts == nil {
		in.Mounts = []dockerInspectMount{}
	}
	// Sort the mounts for deterministic output.
	sort.Slice(in.Mounts, func(i, j int) bool {
		return in.Mounts[i].Source < in.Mounts[j].Source
	})
	for _, k := range in.Mounts {
		out.Volumes[k.Source] = k.Destination
	}

	return out, warns
}

// convertDockerPort converts a Docker port string to a port number and network
// example: "8080/tcp" -> 8080, "tcp"
//
//	"8080" -> 8080, "tcp"
func convertDockerPort(in string) (uint16, string, error) {
	parts := strings.Split(in, "/")
	switch len(parts) {
	case 1:
		// assume it's a TCP port
		p, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", xerrors.Errorf("invalid port format: %s", in)
		}
		return uint16(p), "tcp", nil
	case 2:
		p, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", xerrors.Errorf("invalid port format: %s", in)
		}
		return uint16(p), parts[1], nil
	default:
		return 0, "", xerrors.Errorf("invalid port format: %s", in)
	}
}
