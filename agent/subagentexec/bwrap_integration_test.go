//go:build linux

package subagentexec

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// This test runs the checked-in bubblewrap reference driver for real and then
// inspects the sandbox from the inside. It proves the properties the command
// tests can only assert as arguments: that the shared project really is the
// one writable host directory, that the parent's private files really are
// absent, and that the launcher really reclaims the token afterwards.
//
// It needs bubblewrap, jq, a statically linked BusyBox, and permitted
// unprivileged user namespaces, so it skips with an explicit reason when the
// machine cannot host a sandbox. Once the probe below succeeds, nothing is
// skipped: every property is a hard requirement.

// bwrapSandboxTools are the host tools the reference driver resolves.
type bwrapSandboxTools struct {
	bwrap   string
	jq      string
	busybox string
}

// path returns the controlled PATH the driver must run with: the launcher's
// default plus the directories the probed tools were actually found in, so a
// machine that installs them outside the default locations still exercises the
// driver rather than skipping.
func (tools bwrapSandboxTools) path() string {
	seen := map[string]bool{}
	for _, entry := range filepath.SplitList(DefaultPath) {
		seen[entry] = true
	}
	entries := []string{DefaultPath}
	for _, tool := range []string{tools.bwrap, tools.jq, tools.busybox} {
		dir := filepath.Dir(tool)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries = append(entries, dir)
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

// probeBwrapSandbox skips the test unless this machine can host the sandbox
// the reference driver builds.
//
// The probe is the driver's own hardest requirement in miniature: BusyBox is
// the only executable in an otherwise empty root, in a fresh user and pid
// namespace with no capabilities. It therefore fails on a machine without
// unprivileged user namespaces and on a machine whose BusyBox is dynamically
// linked, which are exactly the two deployment prerequisites.
func probeBwrapSandbox(t *testing.T) bwrapSandboxTools {
	t.Helper()

	var tools bwrapSandboxTools
	for _, tool := range []struct {
		names []string
		into  *string
	}{
		{names: []string{"bwrap"}, into: &tools.bwrap},
		{names: []string{"jq"}, into: &tools.jq},
		// Distributions ship the static build under either name.
		{names: []string{"busybox", "busybox-static"}, into: &tools.busybox},
	} {
		for _, name := range tool.names {
			if resolved, err := exec.LookPath(name); err == nil {
				*tool.into = resolved
				break
			}
		}
		if *tool.into == "" {
			t.Skipf("bubblewrap sandbox unavailable: none of %s is installed", strings.Join(tool.names, ", "))
		}
	}

	probe := exec.Command(tools.bwrap,
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--unshare-cgroup-try", "--die-with-parent", "--new-session",
		"--cap-drop", "ALL", "--clearenv",
		"--uid", "1000", "--gid", "1000",
		"--proc", "/proc", "--dev", "/dev",
		"--ro-bind", tools.busybox, "/bin/busybox",
		"--symlink", "/bin/busybox", "/bin/sh",
		"--", "/bin/busybox", "true",
	)
	if output, err := probe.CombinedOutput(); err != nil {
		t.Skipf("bubblewrap sandbox unavailable: user namespace probe failed: %v: %s", err, output)
	}
	return tools
}

// bwrapSandboxFixture is the host state one sandbox run is judged against.
type bwrapSandboxFixture struct {
	// shared is the project directory the declaration shares with the child.
	shared string
	// parentHome holds the fixtures the child must not be able to reach.
	parentHome string
	// absent are host paths that must not resolve inside the sandbox.
	absent []string
	// coder is the fake Coder binary the driver binds as the child's
	// executable.
	coder string
}

// seedBwrapSandboxFixture creates the shared project plus the parent-side
// files a sandbox escape would expose: a shell dotfile, an SSH private key, an
// unrelated private file, a live Unix socket, and a Docker socket marker.
func seedBwrapSandboxFixture(t *testing.T, root string) *bwrapSandboxFixture {
	t.Helper()

	f := &bwrapSandboxFixture{
		shared:     filepath.Join(root, "project"),
		parentHome: filepath.Join(root, "parent-home"),
		coder:      filepath.Join(root, "coder"),
	}
	sshDir := filepath.Join(f.parentHome, ".ssh")
	require.NoError(t, os.MkdirAll(f.shared, 0o700))
	require.NoError(t, os.MkdirAll(sshDir, 0o700))

	dotfile := filepath.Join(f.parentHome, ".bashrc")
	sshKey := filepath.Join(sshDir, "id_ed25519")
	privateFile := filepath.Join(f.parentHome, "unrelated-secret.txt")
	dockerSocket := filepath.Join(f.parentHome, "docker.sock")
	for path, content := range map[string]string{
		dotfile:      "export PARENT=1\n",
		sshKey:       "PARENT PRIVATE KEY\n",
		privateFile:  "parent only\n",
		dockerSocket: "docker socket marker\n",
	} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	// A real listening Unix socket, so the check is against something the
	// child could actually have connected to.
	agentSocket := filepath.Join(f.parentHome, "agent.sock")
	listener, err := net.Listen("unix", agentSocket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	f.absent = []string{
		f.parentHome, dotfile, sshDir, sshKey, privateFile, dockerSocket, agentSocket,
		// The host's real Docker socket, when it has one.
		"/var/run/docker.sock",
	}

	// The seed the child reads to prove the shared project is bound, and the
	// files the child writes into it to prove the bind is read-write.
	require.NoError(t, os.WriteFile(filepath.Join(f.shared, "seeded.txt"), []byte("from the owner"), 0o600))
	return f
}

// writeFakeCoderBinary writes the executable that stands in for the Coder
// binary. The driver binds it read-only at /opt/coder/coder and the sandbox
// runs it as the child agent, so it reports the sandbox from the inside
// instead of connecting to a deployment.
//
// It runs as /bin/sh, which is a BusyBox applet inside the sandbox, and uses
// only applets the driver exposes.
func writeFakeCoderBinary(t *testing.T, f *bwrapSandboxFixture, absent []string) {
	t.Helper()

	// Every host path is single quoted. They are all test temporary
	// directories, so they contain no quote characters.
	var absentChecks strings.Builder
	for _, path := range absent {
		fmt.Fprintf(&absentChecks, "check_absent '%s'\n", path)
	}

	script := `#!/bin/sh
# Fake Coder binary for the bubblewrap driver's isolation test. It records what
# the sandbox looks like from the inside, waits for the host to hand it a file
# through the shared project, and then stays in the foreground until the
# launcher stops it.
set -u

report="$PWD/report.txt"
: >"$report"

say() {
	printf '%s\n' "$*" >>"$report"
}

yesno() {
	if [ "$1" -eq 0 ]; then printf 'yes'; else printf 'no'; fi
}

check_absent() {
	if [ -e "$1" ] || [ -L "$1" ]; then
		say "leak=$1"
	fi
}

say "argv=$*"
say "pwd=$PWD"
say "uid=$(id -u)"
say "gid=$(id -g)"
say "whoami=$(whoami)"
say "hostname=$(hostname)"
say "home=$HOME"
say "shell=$SHELL"
say "tmpdir=$TMPDIR"
say "xdg_runtime_dir=$XDG_RUNTIME_DIR"
say "path=$PATH"
say "agent_url=$CODER_AGENT_URL"
say "agent_auth=$CODER_AGENT_AUTH"
say "agent_token_file=$CODER_AGENT_TOKEN_FILE"
say "agent_token_env=${CODER_AGENT_TOKEN:-unset}"

say "netns=$(readlink /proc/self/ns/net)"
say "pidns=$(readlink /proc/self/ns/pid)"
say "userns=$(readlink /proc/self/ns/user)"

# The token is present at the fixed path and cannot be written to.
[ -r /run/coder/token ]
say "token_readable=$(yesno $?)"
if printf 'x' >>/run/coder/token 2>/dev/null; then
	say "token_writable=yes"
else
	say "token_writable=no"
fi

# The Coder binary is present and cannot be replaced.
[ -x /opt/coder/coder ]
say "coder_executable=$(yesno $?)"
if printf 'x' >>/opt/coder/coder 2>/dev/null; then
	say "coder_writable=yes"
else
	say "coder_writable=no"
fi

# A private empty root: no host library or interpreter directories at all.
for entry in /usr /lib /lib64 /sbin /var /root /etc/ssh; do
	if [ -e "$entry" ]; then
		say "root_entry=$entry"
	fi
done
say "root_listing=$(ls -A / | sort | tr '\n' ' ')"

# A fresh /proc and a minimal /dev.
[ -r /proc/self/status ]
say "proc_readable=$(yesno $?)"
say "proc_pid_count=$(ls /proc | grep -c '^[0-9][0-9]*$')"
[ -c /dev/null ] && [ -c /dev/zero ] && [ -c /dev/full ] &&
	[ -c /dev/random ] && [ -c /dev/urandom ] && [ -c /dev/tty ]
say "dev_minimal=$(yesno $?)"
say "dev_listing=$(ls -A /dev | sort | tr '\n' ' ')"

# Separate private home, temporary, and runtime directories: a marker written
# to each one must not show up in the shared project.
printf 'home\n' >/home/coder/private-marker
printf 'tmp\n' >/tmp/private-marker
printf 'run\n' >"$XDG_RUNTIME_DIR/private-marker"
[ ! -e "$PWD/private-marker" ]
say "private_dirs_separate=$(yesno $?)"

# The shared project is bound read-write in both directions.
say "seeded=$(cat "$PWD/seeded.txt")"
printf 'from the child\n' >"$PWD/child-wrote.txt"
say "child_write=$(yesno $?)"

# Nothing the parent kept outside the shared project resolves in here.
` + absentChecks.String() + `
say "done=yes"

# Signal the host, then prove the bind is live by echoing back whatever the
# host writes into the shared project.
: >"$PWD/ready"
attempt=0
while [ "$attempt" -lt 120 ]; do
	if [ -f "$PWD/from-host.txt" ]; then
		cat "$PWD/from-host.txt" >"$PWD/echoed.txt"
		break
	fi
	attempt=$((attempt + 1))
	sleep 1
done

# Stay in the foreground: the launcher ends this process.
while true; do
	sleep 1
done
`
	require.NoError(t, os.WriteFile(f.coder, []byte(script), 0o700))
}

// bwrapSandboxReport is the parsed report the fake Coder binary wrote from
// inside the sandbox.
type bwrapSandboxReport struct {
	fields map[string]string
	leaks  []string
	root   []string
}

func parseBwrapSandboxReport(t *testing.T, content string) bwrapSandboxReport {
	t.Helper()

	report := bwrapSandboxReport{fields: map[string]string{}}
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		require.True(t, ok, "unparsable report line %q", line)
		switch key {
		case "leak":
			report.leaks = append(report.leaks, value)
		case "root_entry":
			report.root = append(report.root, value)
		default:
			report.fields[key] = value
		}
	}
	sort.Strings(report.leaks)
	sort.Strings(report.root)
	return report
}

func (r bwrapSandboxReport) field(t *testing.T, name string) string {
	t.Helper()

	value, ok := r.fields[name]
	require.True(t, ok, "the sandbox report has no %s field", name)
	return value
}

// TestBwrapDriverFakeCoderBinarySyntax keeps the sandbox inspection script
// honest on machines that cannot host a sandbox. The isolation test below is
// the only thing that runs it, so a syntax error in it would otherwise stay
// invisible until someone ran the suite on a capable machine.
func TestBwrapDriverFakeCoderBinarySyntax(t *testing.T) {
	t.Parallel()

	fixture := seedBwrapSandboxFixture(t, t.TempDir())
	writeFakeCoderBinary(t, fixture, fixture.absent)

	output, err := exec.Command("sh", "-n", fixture.coder).CombinedOutput()
	require.NoError(t, err, "sh -n rejected the sandbox inspection script: %s", output)
}

func TestBwrapDriverSandboxIsolation(t *testing.T) {
	t.Parallel()

	tools := probeBwrapSandbox(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	root := t.TempDir()
	fixture := seedBwrapSandboxFixture(t, root)

	// The state root is created up front so the private paths the sandbox is
	// checked against are the ones the launcher will really use.
	stateRoot := filepath.Join(root, "state")
	require.NoError(t, os.MkdirAll(stateRoot, privateDirMode))

	driver := newTestDriver(t, func(cfg *ScriptDriverConfig) {
		cfg.StateRoot = stateRoot
		cfg.CoderBinaryPath = fixture.coder
		cfg.Path = tools.path()
	})

	script, err := os.ReadFile(bwrapDriverScript(t))
	require.NoError(t, err)

	launch := testDriverLaunch(t, string(script))
	launch.Declaration.SharedHostPath = fixture.shared
	launch.SharedHostPath = fixture.shared
	paths := driverPaths(t, driver, launch.Declaration.ExecutionID)

	// The launcher's private state, including the token file, must be
	// unreachable from inside the sandbox as well.
	writeFakeCoderBinary(t, fixture, append(append([]string{}, fixture.absent...),
		stateRoot, paths.dir, paths.token, paths.driver, paths.home, paths.tmp, paths.runtime))

	proc, err := driver.Start(ctx, launch)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Stop(ctx) })

	// The child signals readiness by creating this file in the shared
	// project, which is the only place it can write to that the host reads.
	readyPath := filepath.Join(fixture.shared, "ready")
	require.True(t, testutil.Eventually(ctx, t, func(context.Context) bool {
		_, statErr := os.Stat(readyPath)
		return statErr == nil
	}, testutil.IntervalFast), "the sandboxed child never reported")

	reported, err := os.ReadFile(filepath.Join(fixture.shared, "report.txt"))
	require.NoError(t, err)
	report := parseBwrapSandboxReport(t, string(reported))

	// The whole inspection ran, so every check below reflects a complete
	// report rather than one that stopped early.
	require.Equal(t, "yes", report.field(t, "done"))

	// Nothing the parent kept outside the shared project is reachable, and
	// the launcher's private state, including the token file's host path, is
	// not either.
	require.Empty(t, report.leaks, "the sandbox reached host paths it must not see")

	// The host root and the directories that would carry the host's
	// libraries, interpreters, and service configuration are simply absent.
	require.Empty(t, report.root, "the sandbox root exposes host directories")

	// The child ran as the fixed sandbox account, in its own pid, user, and
	// UTS namespaces.
	require.Equal(t, "1000", report.field(t, "uid"))
	require.Equal(t, "1000", report.field(t, "gid"))
	require.Equal(t, "coder", report.field(t, "whoami"))
	require.Equal(t, "coder-subagent", report.field(t, "hostname"))
	hostPidNS, err := os.Readlink("/proc/self/ns/pid")
	require.NoError(t, err)
	require.NotEqual(t, hostPidNS, report.field(t, "pidns"))
	hostUserNS, err := os.Readlink("/proc/self/ns/user")
	require.NoError(t, err)
	require.NotEqual(t, hostUserNS, report.field(t, "userns"))
	// The network namespace is deliberately shared, because the child agent
	// has to reach the deployment.
	hostNetNS, err := os.Readlink("/proc/self/ns/net")
	require.NoError(t, err)
	require.Equal(t, hostNetNS, report.field(t, "netns"))

	// The child reads its token from the fixed read-only path, and the token
	// is not in its environment.
	require.Equal(t, "/run/coder/token", report.field(t, "agent_token_file"))
	require.Equal(t, "yes", report.field(t, "token_readable"))
	require.Equal(t, "no", report.field(t, "token_writable"))
	require.Equal(t, "unset", report.field(t, "agent_token_env"))
	require.Equal(t, "token", report.field(t, "agent_auth"))
	require.Equal(t, driver.coderURL, report.field(t, "agent_url"))
	require.NotContains(t, string(reported), testAuthToken)

	// The Coder binary is available and cannot be replaced, and it was
	// launched as the child agent with its diagnostics listeners disabled,
	// devcontainer detection off, and private state directories.
	require.Equal(t, "yes", report.field(t, "coder_executable"))
	require.Equal(t, "no", report.field(t, "coder_writable"))
	require.Equal(t, "/opt/coder:/bin", report.field(t, "path"))
	argv := report.field(t, "argv")
	for _, expected := range []string{
		"agent",
		"--log-dir /home/coder/.coder-agent/log",
		"--script-data-dir /home/coder/.coder-agent/scripts",
		"--subagent-exec-state-dir /home/coder/.coder-agent/subagent-exec",
		"--pprof-address=",
		"--prometheus-address=",
		"--debug-address=",
		"--devcontainers-enable=false",
	} {
		require.Contains(t, argv, expected)
	}

	// A fresh /proc showing only the sandbox's own processes, and a minimal
	// /dev.
	require.Equal(t, "yes", report.field(t, "proc_readable"))
	require.Equal(t, "yes", report.field(t, "dev_minimal"))
	require.NotContains(t, report.field(t, "dev_listing"), "sda")
	// A fresh /proc in a fresh pid namespace shows only the sandbox's own
	// handful of processes, not the host's.
	pidCount, err := strconv.Atoi(report.field(t, "proc_pid_count"))
	require.NoError(t, err)
	require.LessOrEqual(t, pidCount, 10, "the sandbox sees the host's processes")

	// Private home, temporary, and runtime directories, none of them the
	// shared project.
	require.Equal(t, "/home/coder", report.field(t, "home"))
	require.Equal(t, "/tmp", report.field(t, "tmpdir"))
	require.Equal(t, "/run/user/1000", report.field(t, "xdg_runtime_dir"))
	require.Equal(t, "yes", report.field(t, "private_dirs_separate"))
	// Those markers landed in the launcher's private state, not in the
	// shared project. The child's runtime directory is a sibling of the
	// driver's generated support files, which keeps them off the writable
	// mount.
	for marker, hostPath := range map[string]string{
		"home": filepath.Join(paths.home, "private-marker"),
		"tmp":  filepath.Join(paths.tmp, "private-marker"),
		"run":  filepath.Join(paths.runtime, "xdg", "private-marker"),
	} {
		written, readErr := os.ReadFile(hostPath)
		require.NoError(t, readErr)
		require.Equal(t, marker+"\n", string(written))
	}
	require.NoFileExists(t, filepath.Join(fixture.shared, "private-marker"))

	// The shared project is bound read-write in both directions: the child
	// read what the owner seeded and wrote a file the owner can read.
	require.Equal(t, "from the owner", report.field(t, "seeded"))
	require.Equal(t, "yes", report.field(t, "child_write"))
	childWrote, err := os.ReadFile(filepath.Join(fixture.shared, "child-wrote.txt"))
	require.NoError(t, err)
	require.Equal(t, "from the child\n", string(childWrote))

	// The other direction, while the sandbox is running: a file the host
	// writes now is visible to the child immediately.
	require.NoError(t, os.WriteFile(filepath.Join(fixture.shared, "from-host.txt"),
		[]byte("live from the host\n"), 0o600))
	echoed := filepath.Join(fixture.shared, "echoed.txt")
	require.True(t, testutil.Eventually(ctx, t, func(context.Context) bool {
		echo, statErr := os.ReadFile(echoed)
		return statErr == nil && string(echo) == "live from the host\n"
	}, testutil.IntervalFast), "the shared project bind is not live in both directions")

	// Stopping through the generic launcher's Process ends the sandbox, runs
	// the driver's cleanup, and reclaims the private state and the token.
	require.NoError(t, proc.Stop(ctx))
	require.NoDirExists(t, paths.dir)
	require.NoFileExists(t, paths.token)
	requireNoTokenOnDisk(t, stateRoot, fixture.shared)

	// The owner's project survives the sandbox it hosted.
	require.DirExists(t, fixture.shared)
	seeded, err := os.ReadFile(filepath.Join(fixture.shared, "seeded.txt"))
	require.NoError(t, err)
	require.Equal(t, "from the owner", string(seeded))
}
