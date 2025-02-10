package agentcontainers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
