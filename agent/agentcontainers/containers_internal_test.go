package agentcontainers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestWrapDockerExec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		containerUser string
		cmdArgs       []string
		wantCmd       []string
	}{
		{
			name:          "cmd with no args",
			containerUser: "my-user",
			cmdArgs:       []string{"my-cmd"},
			wantCmd:       []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd"},
		},
		{
			name:          "cmd with args",
			containerUser: "my-user",
			cmdArgs:       []string{"my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
			wantCmd:       []string{"docker", "exec", "--interactive", "--user", "my-user", "my-container", "my-cmd", "arg1", "--arg2", "arg3", "--arg4"},
		},
		{
			name:          "no user specified",
			containerUser: "",
			cmdArgs:       []string{"my-cmd"},
			wantCmd:       []string{"docker", "exec", "--interactive", "my-container", "my-cmd"},
		},
	}
	for _, tt := range tests {
		tt := tt // appease the linter even though this isn't needed anymore
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actualCmd, actualArgs := wrapDockerExec("my-container", tt.containerUser, tt.cmdArgs[0], tt.cmdArgs[1:]...)
			assert.Equal(t, tt.wantCmd[0], actualCmd)
			assert.Equal(t, tt.wantCmd[1:], actualArgs)
		})
	}
}

func TestConvertDockerPort(t *testing.T) {
	t.Parallel()

	//nolint:paralleltest // variable recapture no longer required
	for _, tc := range []struct {
		name          string
		in            string
		expectPort    uint16
		expectNetwork string
		expectError   string
	}{
		{
			name:        "empty port",
			in:          "",
			expectError: "invalid port",
		},
		{
			name:          "valid tcp port",
			in:            "8080/tcp",
			expectPort:    8080,
			expectNetwork: "tcp",
		},
		{
			name:          "valid udp port",
			in:            "8080/udp",
			expectPort:    8080,
			expectNetwork: "udp",
		},
		{
			name:          "valid port no network",
			in:            "8080",
			expectPort:    8080,
			expectNetwork: "tcp",
		},
		{
			name:        "invalid port",
			in:          "invalid/tcp",
			expectError: "invalid port",
		},
		{
			name:        "invalid port no network",
			in:          "invalid",
			expectError: "invalid port",
		},
		{
			name:        "multiple network",
			in:          "8080/tcp/udp",
			expectError: "invalid port",
		},
	} {
		//nolint: paralleltest // variable recapture no longer required
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actualPort, actualNetwork, actualErr := convertDockerPort(tc.in)
			if tc.expectError != "" {
				assert.Zero(t, actualPort, "expected no port")
				assert.Empty(t, actualNetwork, "expected no network")
				assert.ErrorContains(t, actualErr, tc.expectError)
			} else {
				assert.NoError(t, actualErr, "expected no error")
				assert.Equal(t, tc.expectPort, actualPort, "expected port to match")
				assert.Equal(t, tc.expectNetwork, actualNetwork, "expected network to match")
			}
		})
	}
}

func TestConvertDockerVolume(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                string
		in                  string
		expectHostPath      string
		expectContainerPath string
		expectError         string
	}{
		{
			name:        "empty volume",
			in:          "",
			expectError: "invalid volume",
		},
		{
			name:                "length 1 volume",
			in:                  "/path/to/something",
			expectHostPath:      "/path/to/something",
			expectContainerPath: "/path/to/something",
		},
		{
			name:                "length 2 volume",
			in:                  "/path/to/something=/path/to/something/else",
			expectHostPath:      "/path/to/something",
			expectContainerPath: "/path/to/something/else",
		},
		{
			name:        "invalid length volume",
			in:          "/path/to/something=/path/to/something/else=/path/to/something/else/else",
			expectError: "invalid volume",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
		})
	}
}

// TestConvertDockerInspect tests the convertDockerInspect function using
// fixtures from ./testdata.
func TestConvertDockerInspect(t *testing.T) {
	t.Parallel()

	//nolint:paralleltest // variable recapture no longer required
	for _, tt := range []struct {
		name        string
		expect      []codersdk.WorkspaceAgentContainer
		expectWarns []string
		expectError string
	}{
		{
			name: "container_simple",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 55, 58, 91280203, time.UTC),
					ID:           "6b539b8c60f5230b8b0fde2502cd2332d31c0d526a3e6eb6eef1cc39439b3286",
					FriendlyName: "eloquent_kowalevski",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports:        []codersdk.WorkspaceAgentContainerPort{},
					Volumes:      map[string]string{},
				},
			},
		},
		{
			name: "container_labels",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 20, 3, 28, 71706536, time.UTC),
					ID:           "bd8818e670230fc6f36145b21cf8d6d35580355662aa4d9fe5ae1b188a4c905f",
					FriendlyName: "fervent_bardeen",
					Image:        "debian:bookworm",
					Labels:       map[string]string{"baz": "zap", "foo": "bar"},
					Running:      true,
					Status:       "running",
					Ports:        []codersdk.WorkspaceAgentContainerPort{},
					Volumes:      map[string]string{},
				},
			},
		},
		{
			name: "container_binds",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 58, 43, 522505027, time.UTC),
					ID:           "fdc75ebefdc0243c0fce959e7685931691ac7aede278664a0e2c23af8a1e8d6a",
					FriendlyName: "silly_beaver",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports:        []codersdk.WorkspaceAgentContainerPort{},
					Volumes: map[string]string{
						"/tmp/test/a": "/var/coder/a",
						"/tmp/test/b": "/var/coder/b",
					},
				},
			},
		},
		{
			name: "container_sameport",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 56, 34, 842164541, time.UTC),
					ID:           "4eac5ce199d27b2329d0ff0ce1a6fc595612ced48eba3669aadb6c57ebef3fa2",
					FriendlyName: "modest_varahamihira",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports: []codersdk.WorkspaceAgentContainerPort{
						{
							Network:  "tcp",
							Port:     12345,
							HostPort: 12345,
							HostIP:   "0.0.0.0",
						},
					},
					Volumes: map[string]string{},
				},
			},
		},
		{
			name: "container_differentport",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 57, 8, 862545133, time.UTC),
					ID:           "3090de8b72b1224758a94a11b827c82ba2b09c45524f1263dc4a2d83e19625ea",
					FriendlyName: "boring_ellis",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports: []codersdk.WorkspaceAgentContainerPort{
						{
							Network:  "tcp",
							Port:     23456,
							HostPort: 12345,
							HostIP:   "0.0.0.0",
						},
					},
					Volumes: map[string]string{},
				},
			},
		},
		{
			name: "container_sameportdiffip",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 56, 34, 842164541, time.UTC),
					ID:           "a",
					FriendlyName: "a",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports: []codersdk.WorkspaceAgentContainerPort{
						{
							Network:  "tcp",
							Port:     8001,
							HostPort: 8000,
							HostIP:   "0.0.0.0",
						},
					},
					Volumes: map[string]string{},
				},
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 56, 34, 842164541, time.UTC),
					ID:           "b",
					FriendlyName: "b",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports: []codersdk.WorkspaceAgentContainerPort{
						{
							Network:  "tcp",
							Port:     8001,
							HostPort: 8000,
							HostIP:   "::",
						},
					},
					Volumes: map[string]string{},
				},
			},
			expectWarns: []string{"host port 8000 is mapped to multiple containers on different interfaces: a, b"},
		},
		{
			name: "container_volume",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 59, 42, 39484134, time.UTC),
					ID:           "b3688d98c007f53402a55e46d803f2f3ba9181d8e3f71a2eb19b392cf0377b4e",
					FriendlyName: "upbeat_carver",
					Image:        "debian:bookworm",
					Labels:       map[string]string{},
					Running:      true,
					Status:       "running",
					Ports:        []codersdk.WorkspaceAgentContainerPort{},
					Volumes: map[string]string{
						"/var/lib/docker/volumes/testvol/_data": "/testvol",
					},
				},
			},
		},
		{
			name: "devcontainer_simple",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 1, 5, 751972661, time.UTC),
					ID:           "0b2a9fcf5727d9562943ce47d445019f4520e37a2aa7c6d9346d01af4f4f9aed",
					FriendlyName: "optimistic_hopper",
					Image:        "debian:bookworm",
					Labels: map[string]string{
						"devcontainer.config_file": "/home/coder/src/coder/coder/agent/agentcontainers/testdata/devcontainer_simple.json",
						"devcontainer.metadata":    "[]",
					},
					Running: true,
					Status:  "running",
					Ports:   []codersdk.WorkspaceAgentContainerPort{},
					Volumes: map[string]string{},
				},
			},
		},
		{
			name: "devcontainer_forwardport",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 3, 55, 22053072, time.UTC),
					ID:           "4a16af2293fb75dc827a6949a3905dd57ea28cc008823218ce24fab1cb66c067",
					FriendlyName: "serene_khayyam",
					Image:        "debian:bookworm",
					Labels: map[string]string{
						"devcontainer.config_file": "/home/coder/src/coder/coder/agent/agentcontainers/testdata/devcontainer_forwardport.json",
						"devcontainer.metadata":    "[]",
					},
					Running: true,
					Status:  "running",
					Ports:   []codersdk.WorkspaceAgentContainerPort{},
					Volumes: map[string]string{},
				},
			},
		},
		{
			name: "devcontainer_appport",
			expect: []codersdk.WorkspaceAgentContainer{
				{
					CreatedAt:    time.Date(2025, 3, 11, 17, 2, 42, 613747761, time.UTC),
					ID:           "52d23691f4b954d083f117358ea763e20f69af584e1c08f479c5752629ee0be3",
					FriendlyName: "suspicious_margulis",
					Image:        "debian:bookworm",
					Labels: map[string]string{
						"devcontainer.config_file": "/home/coder/src/coder/coder/agent/agentcontainers/testdata/devcontainer_appport.json",
						"devcontainer.metadata":    "[]",
					},
					Running: true,
					Status:  "running",
					Ports: []codersdk.WorkspaceAgentContainerPort{
						{
							Network:  "tcp",
							Port:     8080,
							HostPort: 32768,
							HostIP:   "0.0.0.0",
						},
					},
					Volumes: map[string]string{},
				},
			},
		},
	} {
		// nolint:paralleltest // variable recapture no longer required
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bs, err := os.ReadFile(filepath.Join("testdata", tt.name, "docker_inspect.json"))
			require.NoError(t, err, "failed to read testdata file")
			actual, warns, err := convertDockerInspect(bs)
			if len(tt.expectWarns) > 0 {
				assert.Len(t, warns, len(tt.expectWarns), "expected warnings")
				for _, warn := range tt.expectWarns {
					assert.Contains(t, warns, warn)
				}
			}
			if tt.expectError != "" {
				assert.Empty(t, actual, "expected no data")
				assert.ErrorContains(t, err, tt.expectError)
				return
			}
			require.NoError(t, err, "expected no error")
			if diff := cmp.Diff(tt.expect, actual); diff != "" {
				t.Errorf("unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
