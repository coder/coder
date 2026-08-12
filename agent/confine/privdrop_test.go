package confine_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/confine"
)

const (
	capSetGID  = 6
	capSetUID  = 7
	capSetPCAP = 8
)

func TestPrivilegeDropPreflightReasons(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux preflight logic is unavailable")
	}

	allCapabilities := ^uint64(0)
	supported := confine.PrivilegeDropPreflight{
		TargetUID:                    2000,
		TargetGID:                    2001,
		DeviceGID:                    2002,
		SupervisorUID:                1000,
		SupervisorGID:                1001,
		UIDMapped:                    true,
		GIDMapped:                    true,
		DeviceGIDMapped:              true,
		SupplementaryGroupsSupported: true,
		EffectiveCapabilities:        allCapabilities,
		PermittedCapabilities:        allCapabilities,
		BoundingCapabilities:         allCapabilities,
		LastCapability:               40,
		ShellExecutable:              true,
		NoNewPrivilegesSupported:     true,
	}

	tests := []struct {
		name       string
		preflight  confine.PrivilegeDropPreflight
		wantReason string
	}{
		{name: "supported", preflight: supported},
		{
			name: "probe failure",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.FailureReason = "lookup target user: unknown user"
			}),
			wantReason: "lookup target user: unknown user",
		},
		{
			name: "root UID",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.TargetUID = 0
			}),
			wantReason: "target UID must not be root",
		},
		{
			name: "root GID",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.TargetGID = 0
			}),
			wantReason: "target GID must not be root",
		},
		{
			name: "supervisor UID",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.TargetUID = p.SupervisorUID
			}),
			wantReason: "target UID matches the supervisor UID",
		},
		{
			name: "supervisor GID",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.TargetGID = p.SupervisorGID
			}),
			wantReason: "target GID matches the supervisor GID",
		},
		{
			name: "UID not mapped",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.UIDMapped = false
			}),
			wantReason: "target UID 2000 is not mapped",
		},
		{
			name: "GID not mapped",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.GIDMapped = false
			}),
			wantReason: "target GID 2001 is not mapped",
		},
		{
			name: "device GID not mapped",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.DeviceGroupRequested = true
				p.DeviceGIDMapped = false
			}),
			wantReason: "device GID 2002 is not mapped",
		},
		{
			name: "supplementary groups denied",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.SupplementaryGroupsSupported = false
			}),
			wantReason: "supplementary groups cannot be changed",
		},
		{
			name: "missing setup capability",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.EffectiveCapabilities &^= uint64(1) << capSetUID
			}),
			wantReason: "missing effective capabilities: 7",
		},
		{
			name: "requested capability not permitted",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.RequestedCapabilities = uint64(1) << 12
				p.PermittedCapabilities &^= uint64(1) << 12
			}),
			wantReason: "requested capabilities are not permitted: 12",
		},
		{
			name: "requested capability outside bounding set",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.RequestedCapabilities = uint64(1) << 12
				p.BoundingCapabilities &^= uint64(1) << 12
			}),
			wantReason: "requested capabilities are outside the bounding set: 12",
		},
		{
			name: "missing shell",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.ShellExecutable = false
			}),
			wantReason: "/bin/sh is missing or not executable",
		},
		{
			name: "no new privileges unsupported",
			preflight: withPreflight(supported, func(p *confine.PrivilegeDropPreflight) {
				p.NoNewPrivilegesSupported = false
			}),
			wantReason: "PR_SET_NO_NEW_PRIVS is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reason := test.preflight.Reason()
			if test.wantReason == "" {
				require.Empty(t, reason)
				require.True(t, test.preflight.Supported())
				return
			}
			require.Contains(t, reason, test.wantReason)
			require.False(t, test.preflight.Supported())
		})
	}
}

func TestConfinedCommandConfiguration(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux command construction is unavailable")
	}

	firstFile, err := os.CreateTemp(t.TempDir(), "first")
	require.NoError(t, err)
	t.Cleanup(func() { _ = firstFile.Close() })
	secondFile, err := os.CreateTemp(t.TempDir(), "second")
	require.NoError(t, err)
	t.Cleanup(func() { _ = secondFile.Close() })

	preflight := supportedPreflight()
	preflight.DeviceGroupRequested = true
	preflight.DeviceGID = 36
	preflight.RequestedCapabilities = uint64(1) << 12

	command, err := confine.ConfinedCommand(t.Context(), confine.PrivilegeDropOptions{
		HelperPath:    "/opt/coder",
		CommandPrefix: []string{"/sbin/ip", "netns", "exec", "sandbox"},
		Script:        "printf confined",
		Env:           []string{"TEST_ENV=value"},
		Dir:           "/tmp",
		ExtraFiles:    []*os.File{firstFile, secondFile},
	}, preflight)
	require.NoError(t, err)
	require.Equal(t, []string{
		"/sbin/ip",
		"netns",
		"exec",
		"sandbox",
		"/opt/coder",
		"privdrop-helper",
	}, command.Args[:6])
	require.Equal(t, []string{"TEST_ENV=value"}, command.Env)
	require.Equal(t, "/tmp", command.Dir)
	require.Equal(t, []*os.File{firstFile, secondFile}, command.ExtraFiles)

	var config struct {
		UID                int    `json:"uid"`
		GID                int    `json:"gid"`
		DeviceGID          *int   `json:"device_gid"`
		Capabilities       uint64 `json:"capabilities"`
		LastCapability     int    `json:"last_capability"`
		AllowedDescriptors []int  `json:"allowed_descriptors"`
		Script             string `json:"script"`
	}
	require.NoError(t, json.Unmarshal([]byte(command.Args[6]), &config))
	require.Equal(t, preflight.TargetUID, config.UID)
	require.Equal(t, preflight.TargetGID, config.GID)
	require.Equal(t, new(36), config.DeviceGID)
	require.Equal(t, preflight.RequestedCapabilities, config.Capabilities)
	require.Equal(t, preflight.LastCapability, config.LastCapability)
	require.Equal(t, []int{0, 1, 2, 3, 4}, config.AllowedDescriptors)
	require.Equal(t, "printf confined", config.Script)
}

func TestPreflightPrivilegeDropNumericIDsWithoutEntries(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux credential resolution is unavailable")
	}

	preflight := confine.PreflightPrivilegeDrop(confine.PrivilegeDropOptions{
		User:  "4000000000",
		Group: "4000000001",
	})
	require.Equal(t, 4000000000, preflight.TargetUID)
	require.Equal(t, 4000000001, preflight.TargetGID)
	require.False(t, preflight.UserEntryFound)
	require.False(t, preflight.GroupEntryFound)
}

func TestPreflightPrivilegeDropInvalidIdentifiers(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux credential resolution is unavailable")
	}

	tests := []struct {
		name       string
		options    confine.PrivilegeDropOptions
		wantReason string
	}{
		{
			name:       "missing user",
			options:    confine.PrivilegeDropOptions{},
			wantReason: "target user is required",
		},
		{
			name: "overflowing numeric user",
			options: confine.PrivilegeDropOptions{
				User: "4294967296",
			},
			wantReason: "invalid target user",
		},
		{
			name: "numeric user without group",
			options: confine.PrivilegeDropOptions{
				User: "4000000000",
			},
			wantReason: "target group is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			preflight := confine.PreflightPrivilegeDrop(test.options)
			require.Contains(t, preflight.Reason(), test.wantReason)
			require.False(t, preflight.Supported())
		})
	}
}

func TestRunPrivilegeDropHelperRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux helper validation is unavailable")
	}

	err := confine.RunPrivilegeDropHelper(nil)
	require.ErrorContains(t, err, "requires one configuration argument")
	err = confine.RunPrivilegeDropHelper([]string{"{"})
	require.ErrorContains(t, err, "decode privilege drop helper configuration")
	err = confine.RunPrivilegeDropHelper([]string{`{"uid":0,"gid":1,"last_capability":40,"allowed_descriptors":[0,1,2]}`})
	require.ErrorContains(t, err, "target UID must be a non-root")
}

func TestPrivilegeDropStub(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux stub test")
	}

	preflight := confine.PreflightPrivilegeDrop(confine.PrivilegeDropOptions{})
	require.False(t, preflight.Supported())
	require.Contains(t, preflight.Reason(), "requires Linux")
	_, err := confine.ConfinedCommand(t.Context(), confine.PrivilegeDropOptions{}, preflight)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = confine.LaunchConfined(t.Context(), confine.PrivilegeDropOptions{})
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, confine.RunPrivilegeDropHelper(nil), errors.ErrUnsupported)
}

func TestPrivilegeDropBehavior(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("Linux behavioral test")
	}
	if os.Geteuid() != 0 {
		t.Skip("behavioral privilege drop test requires root")
	}

	secretDir := t.TempDir()
	require.NoError(t, os.Chmod(secretDir, 0o755))
	secretPath := secretDir + "/parent-secret"
	require.NoError(t, os.WriteFile(secretPath, []byte("secret"), 0o600))

	options := confine.PrivilegeDropOptions{
		User:  "65534",
		Group: "65534",
		Script: strings.Join([]string{
			`if cat "$CODER_PRIVDROP_SECRET" >/dev/null 2>&1; then`,
			`  echo "parent secret was readable" >&2`,
			`  exit 91`,
			`fi`,
			`cat /proc/self/status`,
		}, "\n"),
	}
	preflight := confine.PreflightPrivilegeDrop(options)
	if !preflight.Supported() {
		t.Skipf("root environment cannot perform privilege drop: %s", preflight.Reason())
	}
	configurationCommand, err := confine.ConfinedCommand(t.Context(), options, preflight)
	require.NoError(t, err)
	configuration := configurationCommand.Args[len(configurationCommand.Args)-1]

	executable, err := os.Executable()
	require.NoError(t, err)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := agentexec.DefaultExecer.CommandContext(
		t.Context(),
		executable,
		"-test.run=^TestPrivilegeDropHelperProcess$",
	)
	command.Env = append(os.Environ(),
		"CODER_PRIVDROP_TEST_CONFIG="+configuration,
		"CODER_PRIVDROP_SECRET="+secretPath,
	)
	command.Stdout = &stdout
	command.Stderr = &stderr
	require.NoError(t, command.Run(), stderr.String())

	status := stdout.String()
	require.Contains(t, status, "CapEff:\t0000000000000000")
	require.Contains(t, status, "CapBnd:\t0000000000000000")
	require.Contains(t, status, "NoNewPrivs:\t1")
}

func TestPrivilegeDropHelperProcess(t *testing.T) {
	t.Parallel()
	configuration, ok := os.LookupEnv("CODER_PRIVDROP_TEST_CONFIG")
	if !ok {
		t.Skip("privilege drop helper subprocess")
	}
	if err := confine.RunPrivilegeDropHelper([]string{configuration}); err != nil {
		panic(err)
	}
}

func supportedPreflight() confine.PrivilegeDropPreflight {
	allCapabilities := ^uint64(0)
	return confine.PrivilegeDropPreflight{
		TargetUID:                    2000,
		TargetGID:                    2001,
		SupervisorUID:                1000,
		SupervisorGID:                1001,
		UIDMapped:                    true,
		GIDMapped:                    true,
		DeviceGIDMapped:              true,
		SupplementaryGroupsSupported: true,
		EffectiveCapabilities:        allCapabilities | capabilitySet(capSetGID, capSetUID, capSetPCAP),
		PermittedCapabilities:        allCapabilities,
		BoundingCapabilities:         allCapabilities,
		LastCapability:               40,
		ShellExecutable:              true,
		NoNewPrivilegesSupported:     true,
	}
}

func withPreflight(
	preflight confine.PrivilegeDropPreflight,
	modify func(*confine.PrivilegeDropPreflight),
) confine.PrivilegeDropPreflight {
	modify(&preflight)
	return preflight
}

func capabilitySet(capabilities ...int) uint64 {
	var mask uint64
	for _, capability := range capabilities {
		mask |= uint64(1) << capability
	}
	return mask
}
