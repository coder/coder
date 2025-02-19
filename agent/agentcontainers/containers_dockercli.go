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
	"github.com/coder/coder/v2/codersdk"

	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"
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

// ContainerEnvInfoer is an implementation of agentssh.EnvInfoer that returns
// information about a container.
type ContainerEnvInfoer struct {
	container string
	user      *user.User
	userShell string
	env       []string
}

// EnvInfo returns information about the environment of a container.
func EnvInfo(ctx context.Context, execer agentexec.Execer, container, containerUser string) (*ContainerEnvInfoer, error) {
	var dei ContainerEnvInfoer
	dei.container = container

	var stdoutBuf, stderrBuf bytes.Buffer
	if containerUser == "" {
		// Get the "default" user of the container if no user is specified.
		// TODO: handle different container runtimes.
		cmd, args := WrapDockerExec(container, "")("whoami")
		execCmd := execer.CommandContext(ctx, cmd, args...)
		execCmd.Stdout = &stdoutBuf
		execCmd.Stderr = &stderrBuf
		if err := execCmd.Run(); err != nil {
			return nil, xerrors.Errorf("get container user: run whoami: %w: stderr: %q", err, strings.TrimSpace(stderrBuf.String()))
		}
		out := strings.TrimSpace(stdoutBuf.String())
		if len(out) == 0 {
			return nil, xerrors.Errorf("get container user: run whoami: empty output: stderr: %q", strings.TrimSpace(stderrBuf.String()))
		}
		containerUser = out
		stdoutBuf.Reset()
		stderrBuf.Reset()
	}
	// Now that we know the username, get the required info from the container.
	// We can't assume the presence of `getent` so we'll just have to sniff /etc/passwd.
	cmd, args := WrapDockerExec(container, containerUser)("cat", "/etc/passwd")
	execCmd := execer.CommandContext(ctx, cmd, args...)
	execCmd.Stdout = &stdoutBuf
	execCmd.Stderr = &stderrBuf
	if err := execCmd.Run(); err != nil {
		return nil, xerrors.Errorf("get container user: read /etc/passwd: %w stderr: %q", err, strings.TrimSpace(stderrBuf.String()))
	}

	scanner := bufio.NewScanner(&stdoutBuf)
	var foundLine string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if !strings.HasPrefix(line, containerUser+":") {
			continue
		}
		foundLine = line
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
	if len(passwdFields) < 7 {
		return nil, xerrors.Errorf("get container user: invalid line in /etc/passwd: %q", foundLine)
	}

	// The fourth entry in /etc/passwd contains GECOS information, which is a
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

	// Finally, get the environment of the container.
	stdoutBuf.Reset()
	stderrBuf.Reset()
	cmd, args = WrapDockerExec(container, containerUser)("env")
	execCmd = execer.CommandContext(ctx, cmd, args...)
	execCmd.Stdout = &stdoutBuf
	execCmd.Stderr = &stderrBuf
	if err := execCmd.Run(); err != nil {
		return nil, xerrors.Errorf("get container environment: run env: %w stderr: %q", err, strings.TrimSpace(stderrBuf.String()))
	}

	scanner = bufio.NewScanner(&stdoutBuf)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		dei.env = append(dei.env, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("get container environment: scan env output: %w", err)
	}

	return &dei, nil
}

func (dei *ContainerEnvInfoer) CurrentUser() (*user.User, error) {
	// Clone the user so that the caller can't modify it
	u := &user.User{
		Gid:      dei.user.Gid,
		HomeDir:  dei.user.HomeDir,
		Name:     dei.user.Name,
		Uid:      dei.user.Uid,
		Username: dei.user.Username,
	}
	return u, nil
}

func (dei *ContainerEnvInfoer) Environ() []string {
	// Return a clone of the environment so that the caller can't modify it
	return slices.Clone(dei.env)
}

func (dei *ContainerEnvInfoer) UserHomeDir() (string, error) {
	return dei.user.HomeDir, nil
}

func (dei *ContainerEnvInfoer) UserShell(string) (string, error) {
	return dei.userShell, nil
}

func (dei *ContainerEnvInfoer) ModifyCommand(cmd string, args ...string) (string, []string) {
	return WrapDockerExecPTY(dei.container, dei.user.Username)(cmd, args...)
}

// WrapFn is a function that wraps a command and its arguments with another command and arguments.
type WrapFn func(cmd string, args ...string) (string, []string)

// WrapDockerExec returns a WrapFn that wraps the given command and arguments
// with a docker exec command that runs as the given user in the given container.
func WrapDockerExec(containerName, userName string) WrapFn {
	return func(cmd string, args ...string) (string, []string) {
		dockerArgs := []string{"exec", "--interactive"}
		if userName != "" {
			dockerArgs = append(dockerArgs, "--user", userName)
		}
		dockerArgs = append(dockerArgs, containerName, cmd)
		return "docker", append(dockerArgs, args...)
	}
}

// WrapDockerExecPTY is similar to WrapDockerExec but also allocates a PTY.
func WrapDockerExecPTY(containerName, userName string) WrapFn {
	return func(cmd string, args ...string) (string, []string) {
		dockerArgs := []string{"exec", "--interactive", "--tty"}
		if userName != "" {
			dockerArgs = append(dockerArgs, "--user", userName)
		}
		dockerArgs = append(dockerArgs, containerName, cmd)
		return "docker", append(dockerArgs, args...)
	}
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
	// Run `docker inspect` on each container ID
	stdoutBuf.Reset()
	stderrBuf.Reset()
	// nolint: gosec // We are not executing user input, these IDs come from
	// `docker ps`.
	cmd = dcl.execer.CommandContext(ctx, "docker", append([]string{"inspect"}, ids...)...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("run docker inspect: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
	}

	dockerInspectStderr := strings.TrimSpace(stderrBuf.String())

	// NOTE: There is an unavoidable potential race condition where a
	// container is removed between `docker ps` and `docker inspect`.
	// In this case, stderr will contain an error message but stdout
	// will still contain valid JSON. We will just end up missing
	// information about the removed container. We could potentially
	// log this error, but I'm not sure it's worth it.
	ins := make([]dockerInspect, 0, len(ids))
	if err := json.NewDecoder(&stdoutBuf).Decode(&ins); err != nil {
		// However, if we just get invalid JSON, we should absolutely return an error.
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("decode docker inspect output: %w", err)
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
