package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/coder/v2/codersdk"

	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"
)

// dockerCLIContainerLister is a ContainerLister that lists containers using the docker CLI
type dockerCLIContainerLister struct{}

var _ ContainerLister = &dockerCLIContainerLister{}

func (*dockerCLIContainerLister) List(ctx context.Context) ([]codersdk.WorkspaceAgentContainer, error) {
	var buf bytes.Buffer
	// List all container IDs, one per line, with no truncation
	cmd := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--no-trunc")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, xerrors.Errorf("run docker ps: %w", err)
	}

	ids := make([]string, 0)
	for _, line := range strings.Split(buf.String(), "\n") {
		tmp := strings.TrimSpace(line)
		if tmp == "" {
			continue
		}
		ids = append(ids, tmp)
	}

	// now we can get the detailed information for each container
	// Run `docker inspect` on each container ID
	buf.Reset()
	execArgs := []string{"inspect"}
	execArgs = append(execArgs, ids...)
	cmd = exec.CommandContext(ctx, "docker", execArgs...)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, xerrors.Errorf("run docker inspect: %w", err)
	}

	ins := make([]dockerInspect, 0)
	if err := json.NewDecoder(&buf).Decode(&ins); err != nil {
		return nil, xerrors.Errorf("decode docker inspect output: %w", err)
	}

	out := make([]codersdk.WorkspaceAgentContainer, 0)
	for _, in := range ins {
		out = append(out, convertDockerInspect(in))
	}

	return out, nil
}

// To avoid a direct dependency on the Docker API, we use the docker CLI
// to fetch information about containers.
type dockerInspect struct {
	ID      string              `json:"Id"`
	Created time.Time           `json:"Created"`
	Name    string              `json:"Name"`
	Config  dockerInspectConfig `json:"Config"`
	State   dockerInspectState  `json:"State"`
}

type dockerInspectConfig struct {
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Image        string              `json:"Image"`
	Labels       map[string]string   `json:"Labels"`
	Volumes      map[string]struct{} `json:"Volumes"`
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

func convertDockerInspect(in dockerInspect) codersdk.WorkspaceAgentContainer {
	out := codersdk.WorkspaceAgentContainer{
		CreatedAt: in.Created,
		// Remove the leading slash from the container name
		FriendlyName: strings.TrimPrefix(in.Name, "/"),
		ID:           in.ID,
		Image:        in.Config.Image,
		Labels:       in.Config.Labels,
		Ports:        make([]codersdk.WorkspaceAgentListeningPort, 0),
		Running:      in.State.Running,
		Status:       in.State.String(),
		Volumes:      make(map[string]string),
	}

	// sort the keys for deterministic output
	portKeys := maps.Keys(in.Config.ExposedPorts)
	sort.Strings(portKeys)
	for _, p := range portKeys {
		port, network, err := convertDockerPort(p)
		if err != nil {
			// ignore invalid ports
			continue
		}
		out.Ports = append(out.Ports, codersdk.WorkspaceAgentListeningPort{
			Network: network,
			Port:    port,
		})
	}

	// sort the keys for deterministic output
	volKeys := maps.Keys(in.Config.Volumes)
	sort.Strings(volKeys)
	for _, k := range volKeys {
		v0, v1 := convertDockerVolume(k)
		out.Volumes[v0] = v1
	}

	return out
}

// convertDockerPort converts a Docker port string to a port number and network
// example: "8080/tcp" -> 8080, "tcp"
//
//	"8080" -> 8080, "tcp"
func convertDockerPort(in string) (uint16, string, error) {
	parts := strings.Split(in, "/")
	switch len(parts) {
	case 0:
		return 0, "", xerrors.Errorf("invalid port format: %s", in)
	case 1:
		// assume it's a TCP port
		p, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", xerrors.Errorf("invalid port format: %s", in)
		}
		return uint16(p), "tcp", nil
	default:
		p, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", xerrors.Errorf("invalid port format: %s", in)
		}
		return uint16(p), parts[1], nil
	}
}

// convertDockerVolume converts a Docker volume string to a host path and
// container path. If the host path is not specified, the container path is used
// as the host path.
// example: "/host/path=/container/path" -> "/host/path", "/container/path"
//
//	"/container/path" -> "/container/path", "/container/path"
func convertDockerVolume(in string) (hostPath, containerPath string) {
	parts := strings.Split(in, "=")
	switch len(parts) {
	case 0:
		return in, in
	case 1:
		return parts[0], parts[0]
	default:
		return parts[0], parts[1]
	}
}
