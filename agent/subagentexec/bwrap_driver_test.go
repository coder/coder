//go:build linux

package subagentexec

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The checked-in bubblewrap reference driver is the one driver this
// repository vets. These tests run that exact script against fake bwrap, jq,
// and busybox executables, so they assert the sandbox policy the driver asks
// for without needing bubblewrap, user namespaces, or a static BusyBox on the
// machine running the tests.
//
// Only this script is vetted. A deployment that ships its own driver body is
// shipping a trusted custom integration, and nothing here says anything about
// it.

// bwrapDriverRelPath is the checked-in driver's path relative to this
// package's directory. The tests resolve it from this file's own location, so
// they do not depend on the working directory the test binary runs in.
const bwrapDriverRelPath = "../../examples/templates/x/linux-bwrap-subagent/drivers/bwrap.sh"

// bwrapSentinelName is created in the driver's working directory if any input
// value is ever expanded by a shell. The shared project directory's name
// contains a command substitution that would create it.
const bwrapSentinelName = "expansion-sentinel"

// bwrapHostileName is used for the shared project directory. It carries
// spaces, a command substitution, a backquote substitution, shell operators,
// glob characters, quotes, and a variable reference: if the driver ever
// splices an input value into a command string or leaves it unquoted, one of
// them shows up as a separate argv element, a missing element, or the
// sentinel file.
const bwrapHostileName = "pro ject; $(touch " + bwrapSentinelName + ") & " +
	"`touch " + bwrapSentinelName + "`" + ` | * ? [a-z] $HOME 'q' "d"`

// bwrapFixture is one prepared invocation of the checked-in driver: the fake
// tools it resolves, the private state the launcher would have created, and
// the protocol documents for both operations.
type bwrapFixture struct {
	script string
	// binDir holds the fake bwrap, jq, and busybox. It is the first PATH
	// entry, so it shadows anything the host happens to have installed.
	binDir string
	// argvFile is where the fake bwrap records its NUL-separated argument
	// list.
	argvFile string
	// fixtureDir holds the per-field values the fake jq answers with.
	fixtureDir string
	// cwd is the driver's working directory, kept empty so an accidental
	// shell expansion is visible as a stray file.
	cwd string

	// The host paths the protocol document names.
	stateRoot   string
	statePath   string
	tokenPath   string
	coderPath   string
	sharedPath  string
	homePath    string
	tmpPath     string
	runtimePath string

	// Host paths the sandbox must never expose.
	parentHome   string
	dockerSocket string

	input        DriverInput
	runInput     string
	cleanupInput string
}

// bwrapDriverScript returns the checked-in driver's absolute path, resolved
// from this file's own location.
func bwrapDriverScript(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve this test file's location")
	script, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), bwrapDriverRelPath))
	require.NoError(t, err)
	info, err := os.Stat(script)
	require.NoError(t, err, "the checked-in bubblewrap driver must exist at %s", script)
	require.False(t, info.IsDir())
	return script
}

func newBwrapFixture(t *testing.T) *bwrapFixture {
	t.Helper()

	root := t.TempDir()
	f := &bwrapFixture{
		script:       bwrapDriverScript(t),
		binDir:       filepath.Join(root, "bin"),
		argvFile:     filepath.Join(root, "bwrap-argv"),
		fixtureDir:   filepath.Join(root, "jq-fixtures"),
		cwd:          filepath.Join(root, "cwd"),
		stateRoot:    filepath.Join(root, "state"),
		coderPath:    filepath.Join(root, "coder"),
		sharedPath:   filepath.Join(root, "project", bwrapHostileName),
		parentHome:   filepath.Join(root, "parent-home"),
		dockerSocket: filepath.Join(root, "docker.sock"),
	}
	executionID := uuid.New()
	f.statePath = filepath.Join(f.stateRoot, "agent", executionID.String())
	f.tokenPath = filepath.Join(f.statePath, tokenFileName)
	f.homePath = filepath.Join(f.statePath, homeDirName)
	f.tmpPath = filepath.Join(f.statePath, tmpDirName)
	f.runtimePath = filepath.Join(f.statePath, runtimeDirName)

	for _, dir := range []string{
		f.binDir, f.cwd, f.sharedPath, f.parentHome,
		f.homePath, f.tmpPath, f.runtimePath,
	} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	// The launcher writes the token as a private file; the driver only ever
	// binds its path.
	require.NoError(t, os.WriteFile(f.tokenPath, []byte(testAuthToken), privateFileMode))
	require.NoError(t, os.WriteFile(f.coderPath, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	require.NoError(t, os.WriteFile(f.dockerSocket, []byte("marker"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.parentHome, ".netrc"), []byte("secret"), 0o600))

	f.input = DriverInput{
		ProtocolVersion: ProtocolVersion,
		ExecutionID:     executionID,
		Generation:      uuid.New(),
		ChildAgentID:    uuid.New(),
		ChildAgentName:  "chat; $(touch " + bwrapSentinelName + ")",
		CoderURL:        "https://coder.example.com",
		CoderBinaryPath: f.coderPath,
		TokenFilePath:   f.tokenPath,
		SharedHostPath:  f.sharedPath,
		SharedChildPath: "/workspace/pro ject",
		StatePath:       f.statePath,
		HomePath:        f.homePath,
		TmpPath:         f.tmpPath,
		RuntimePath:     f.runtimePath,
	}

	f.writeFakeTools(t)
	f.runInput = f.writeInput(t, OperationRun)
	f.cleanupInput = f.writeInput(t, OperationCleanup)
	return f
}

// writeInput writes one operation's protocol document and the jq fixtures for
// it. The fixtures are derived from the marshalled document, so they always
// carry exactly the fields the launcher really writes.
func (f *bwrapFixture) writeInput(t *testing.T, op Operation) string {
	t.Helper()

	input := f.input
	input.Operation = op
	document, err := json.Marshal(input)
	require.NoError(t, err)

	path := filepath.Join(f.statePath, string(op)+".json")
	require.NoError(t, os.WriteFile(path, document, privateFileMode))

	var fields map[string]any
	require.NoError(t, json.Unmarshal(document, &fields))
	base := filepath.Join(f.fixtureDir, string(op))
	for _, kind := range []string{"strings", "numbers"} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, kind), 0o700))
	}
	for name, value := range fields {
		kind, text := "strings", ""
		switch typed := value.(type) {
		case string:
			text = typed
		case float64:
			kind, text = "numbers", strconv.FormatFloat(typed, 'f', -1, 64)
		default:
			t.Fatalf("unexpected type %T for protocol field %s", value, name)
		}
		require.NoError(t, os.WriteFile(filepath.Join(base, kind, name), []byte(text), 0o600))
	}
	return path
}

// writeFakeTools installs the fake bwrap, jq, and busybox. Every path they
// need is written into them, so they work with the environment the launcher
// builds from scratch rather than depending on extra variables.
func (f *bwrapFixture) writeFakeTools(t *testing.T) {
	t.Helper()

	// The fake bwrap records its argument list NUL separated. NUL is the one
	// byte a path cannot contain, so the recording proves how many argv
	// elements each value became.
	fakeBwrap := "#!/bin/sh\n" +
		": >'" + f.argvFile + "'\n" +
		"for arg in \"$@\"; do printf '%s\\0' \"$arg\" >>'" + f.argvFile + "'; done\n"

	// The fake jq deliberately does not parse JSON. The test writes one
	// fixture file per field, so a value containing spaces, quotes, or shell
	// metacharacters is handed back byte for byte. It picks the string or the
	// number fixture from the filter the driver passes, which is the same way
	// real jq distinguishes the two type checks.
	fakeJQ := "#!/bin/sh\n" +
		"field=''\nkind=strings\nprev=''\nop=''\n" +
		"for arg in \"$@\"; do\n" +
		"\tif [ \"$prev\" = field ]; then field=\"$arg\"; fi\n" +
		"\tcase \"$arg\" in *tostring*) kind=numbers ;; esac\n" +
		"\tcase \"$arg\" in */run.json) op=run ;; */cleanup.json) op=cleanup ;; esac\n" +
		"\tprev=\"$arg\"\n" +
		"done\n" +
		"if [ -z \"$field\" ] || [ -z \"$op\" ]; then\n" +
		"\techo 'fake jq: unexpected invocation' >&2\n\texit 1\nfi\n" +
		"file='" + f.fixtureDir + "'/\"$op\"/\"$kind\"/\"$field\"\n" +
		"if [ -f \"$file\" ]; then cat \"$file\"; fi\n" +
		"printf '\\n'\n"

	for name, body := range map[string]string{
		"bwrap":   fakeBwrap,
		"jq":      fakeJQ,
		"busybox": "#!/bin/sh\nexit 0\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(f.binDir, name), []byte(body), 0o700))
	}
}

// run invokes the checked-in driver exactly the way the launcher does: the
// script directly, two arguments, and an environment built from scratch whose
// PATH puts the fake tools first.
func (f *bwrapFixture) run(t *testing.T, op Operation, inputPath string) (string, error) {
	t.Helper()

	cmd := exec.Command(f.script, string(op), inputPath)
	cmd.Dir = f.cwd
	cmd.Env = []string{
		"PATH=" + f.binDir + string(os.PathListSeparator) + DefaultPath,
		"HOME=" + f.homePath,
		"TMPDIR=" + f.tmpPath,
		"XDG_RUNTIME_DIR=" + f.runtimePath,
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// requireRun runs the operation and requires it to succeed.
func (f *bwrapFixture) requireRun(t *testing.T, op Operation, inputPath string) string {
	t.Helper()

	output, err := f.run(t, op, inputPath)
	require.NoError(t, err, "driver %s failed: %s", op, output)
	return output
}

// argv returns the argument list the fake bwrap recorded.
func (f *bwrapFixture) argv(t *testing.T) bwrapArgv {
	t.Helper()

	recorded, err := os.ReadFile(f.argvFile)
	require.NoError(t, err, "the driver must have executed bwrap")
	elements := strings.Split(string(recorded), "\x00")
	// The recording ends with a trailing separator.
	require.Equal(t, "", elements[len(elements)-1])
	return bwrapArgv(elements[:len(elements)-1])
}

// bwrapArgv is one recorded bubblewrap argument list.
type bwrapArgv []string

// has reports whether the flag appears as its own element.
func (a bwrapArgv) has(flag string) bool {
	for _, element := range a {
		if element == flag {
			return true
		}
	}
	return false
}

// count returns how many times value appears as its own element.
func (a bwrapArgv) count(value string) int {
	total := 0
	for _, element := range a {
		if element == value {
			total++
		}
	}
	return total
}

// value returns the single element following flag.
func (a bwrapArgv) value(t *testing.T, flag string) string {
	t.Helper()

	values := a.values(flag, 1)
	require.Len(t, values, 1, "expected exactly one %s", flag)
	return values[0][0]
}

// values returns the arity elements following every occurrence of flag. An
// occurrence without enough elements after it is reported as a failure rather
// than silently skipped, because it would mean the driver emitted a truncated
// mount.
func (a bwrapArgv) values(flag string, arity int) [][]string {
	found := [][]string{}
	for i, element := range a {
		if element != flag {
			continue
		}
		if i+arity >= len(a) {
			return append(found, nil)
		}
		found = append(found, a[i+1:i+1+arity])
	}
	return found
}

// pairs returns the source and destination of every occurrence of a two
// argument mount flag.
func (a bwrapArgv) pairs(flag string) [][]string {
	return a.values(flag, 2)
}

// setenv returns the value the sandbox environment variable is set to.
func (a bwrapArgv) setenv(t *testing.T, name string) string {
	t.Helper()

	for _, pair := range a.pairs("--setenv") {
		if pair[0] == name {
			return pair[1]
		}
	}
	t.Fatalf("--setenv %s is missing from %v", name, a)
	return ""
}

// command returns the child command line, which follows the single end of
// options separator.
func (a bwrapArgv) command(t *testing.T) []string {
	t.Helper()

	require.Equal(t, 1, a.count("--"), "expected exactly one end of options separator")
	for i, element := range a {
		if element == "--" {
			return a[i+1:]
		}
	}
	return nil
}

// The bubblewrap flags that map a host path into the sandbox. The read-write
// ones are listed separately, because exactly one host directory may be
// writable.
var (
	bwrapReadWriteBindFlags = []string{"--bind", "--bind-try", "--dev-bind", "--dev-bind-try"}
	bwrapReadOnlyBindFlags  = []string{"--ro-bind", "--ro-bind-try"}
)

// hostBinds returns every host path the argument list maps into the sandbox,
// paired with its destination, for the given flags.
func (a bwrapArgv) hostBinds(flags ...string) [][]string {
	var binds [][]string
	for _, flag := range flags {
		binds = append(binds, a.pairs(flag)...)
	}
	return binds
}

// bwrapFlagArity is the number of arguments each bubblewrap flag consumes.
// The recorded argument list is walked with this table rather than by
// searching for flag names, so a mount whose source or destination happens to
// look like a flag cannot be read as one, and a flag this table does not know
// is reported instead of being skipped: an unknown flag has an unknown arity,
// which would desynchronise the walk and could hide a host mount behind it.
var bwrapFlagArity = map[string]int{
	// Namespaces, privileges, and process setup. None of them names a host
	// path.
	"--unshare-all":            0,
	"--unshare-user":           0,
	"--unshare-user-try":       0,
	"--unshare-ipc":            0,
	"--unshare-pid":            0,
	"--unshare-net":            0,
	"--unshare-uts":            0,
	"--unshare-cgroup":         0,
	"--unshare-cgroup-try":     0,
	"--share-net":              0,
	"--disable-userns":         0,
	"--assert-userns-disabled": 0,
	"--as-pid-1":               0,
	"--die-with-parent":        0,
	"--new-session":            0,
	"--level-prefix":           0,
	"--clearenv":               0,
	"--userns":                 1,
	"--userns2":                1,
	"--pidns":                  1,
	"--uid":                    1,
	"--gid":                    1,
	"--hostname":               1,
	"--chdir":                  1,
	"--argv0":                  1,
	"--args":                   1,
	"--cap-add":                1,
	"--cap-drop":               1,
	"--seccomp":                1,
	"--add-seccomp-fd":         1,
	"--sync-fd":                1,
	"--info-fd":                1,
	"--json-status-fd":         1,
	"--block-fd":               1,
	"--userns-block-fd":        1,
	"--exec-label":             1,
	"--file-label":             1,
	"--unsetenv":               1,
	"--setenv":                 2,

	// Filesystem construction inside the sandbox. These create or adjust
	// sandbox-owned objects and expose nothing from the host.
	"--proc":        1,
	"--dev":         1,
	"--tmpfs":       1,
	"--mqueue":      1,
	"--dir":         1,
	"--lock-file":   1,
	"--remount-ro":  1,
	"--perms":       1,
	"--size":        1,
	"--tmp-overlay": 1,
	"--ro-overlay":  1,
	"--chmod":       2,
	"--symlink":     2,

	// Everything that can put host content into the sandbox, including the
	// variants this driver must never use.
	"--bind":         2,
	"--bind-try":     2,
	"--ro-bind":      2,
	"--ro-bind-try":  2,
	"--dev-bind":     2,
	"--dev-bind-try": 2,
	"--overlay-src":  1,
	"--overlay":      3,
	"--file":         2,
	"--bind-data":    2,
	"--ro-bind-data": 2,
}

// bwrapHostExposingFlags are the operations that can place host content in the
// sandbox, whether by path or through an inherited file descriptor.
var bwrapHostExposingFlags = map[string]bool{
	"--bind":         true,
	"--bind-try":     true,
	"--ro-bind":      true,
	"--ro-bind-try":  true,
	"--dev-bind":     true,
	"--dev-bind-try": true,
	"--overlay-src":  true,
	"--overlay":      true,
	"--file":         true,
	"--bind-data":    true,
	"--ro-bind-data": true,
}

// bwrapOp is one parsed bubblewrap operation: the flag and exactly the
// arguments it consumed.
type bwrapOp struct {
	flag string
	args []string
}

// bwrapPolicyAbort is what the policy assertion's FailNow panics with when it
// runs against a recorder rather than a real test.
type bwrapPolicyAbort struct{}

// bwrapPolicyRecorder captures the failures an assertion reports instead of
// failing the enclosing test. It lets the policy assertion itself be tested
// against a tampered argument list.
type bwrapPolicyRecorder struct {
	failures []string
}

func (r *bwrapPolicyRecorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (*bwrapPolicyRecorder) FailNow() {
	panic(bwrapPolicyAbort{})
}

// bwrapPolicyFailures runs the mount policy assertion against argv and returns
// the failures it reported, so a test can assert that a mutated argument list
// is rejected.
func bwrapPolicyFailures(f *bwrapFixture, argv bwrapArgv) []string {
	recorder := &bwrapPolicyRecorder{}
	func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if _, ok := recovered.(bwrapPolicyAbort); !ok {
				panic(recovered)
			}
		}()
		requireBwrapMountPolicy(recorder, f, argv)
	}()
	return recorder.failures
}

// bwrapOps parses the recorded argument list into operations, stopping at the
// end of options separator because everything after it is the child command
// line rather than a bubblewrap operation.
func bwrapOps(t require.TestingT, argv bwrapArgv) []bwrapOp {
	ops := []bwrapOp{}
	for i := 0; i < len(argv); {
		element := argv[i]
		if element == "--" {
			break
		}
		arity, known := bwrapFlagArity[element]
		if !known {
			require.Failf(t, "unrecognised bubblewrap operand",
				"element %d is %q, which is not a bubblewrap flag this test knows the arity of", i, element)
			return ops
		}
		if i+1+arity > len(argv) {
			require.Failf(t, "truncated bubblewrap operation",
				"%s needs %d arguments but only %d elements follow it", element, arity, len(argv)-i-1)
			return ops
		}
		ops = append(ops, bwrapOp{
			flag: element,
			args: append([]string(nil), argv[i+1:i+1+arity]...),
		})
		i += 1 + arity
	}
	return ops
}

// bwrapHostFileExists reports whether path is a regular file, which is the
// same condition the driver's `[ -f ... ]` tests apply to its optional host
// files.
func bwrapHostFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// bwrapCABundleCandidates returns the ordered CA bundle list the driver itself
// declares. Reading it from the script keeps the expected mount set from
// drifting away from the candidates the driver actually tries.
func bwrapCABundleCandidates(t require.TestingT, script string) []string {
	body, err := os.ReadFile(script)
	require.NoError(t, err)

	const marker = "readonly ca_bundle_candidates='"
	start := strings.Index(string(body), marker)
	require.GreaterOrEqual(t, start, 0, "the driver must declare ca_bundle_candidates")
	rest := string(body)[start+len(marker):]
	end := strings.Index(rest, "'")
	require.GreaterOrEqual(t, end, 0, "the ca_bundle_candidates list must be closed")

	candidates := strings.Fields(rest[:end])
	require.NotEmpty(t, candidates)
	for _, candidate := range candidates {
		require.True(t, strings.HasPrefix(candidate, "/"),
			"CA bundle candidate %s must be an absolute path", candidate)
	}
	return candidates
}

// expectedReadOnlyBinds is the complete set of read-only host binds the driver
// may emit for this fixture: the resolved BusyBox, the support files the
// driver generated, the Coder binary, the child's token file, and the optional
// host resolver and trust files exactly when this machine has them.
func (f *bwrapFixture) expectedReadOnlyBinds(t require.TestingT) [][]string {
	generated := filepath.Join(f.runtimePath, "etc")
	expected := [][]string{
		{filepath.Join(f.binDir, "busybox"), "/bin/busybox"},
		{filepath.Join(generated, "passwd"), "/etc/passwd"},
		{filepath.Join(generated, "group"), "/etc/group"},
		{filepath.Join(generated, "nsswitch.conf"), "/etc/nsswitch.conf"},
		{filepath.Join(generated, "os-release"), "/etc/os-release"},
		{f.coderPath, "/opt/coder/coder"},
		{f.tokenPath, "/run/coder/token"},
	}
	for _, optional := range []string{"/etc/resolv.conf", "/etc/hosts"} {
		if bwrapHostFileExists(optional) {
			expected = append(expected, []string{optional, optional})
		}
	}
	// Only the first candidate that exists is mounted, so a machine with
	// several bundles still gets exactly one.
	for _, candidate := range bwrapCABundleCandidates(t, f.script) {
		if bwrapHostFileExists(candidate) {
			expected = append(expected, []string{candidate, candidate})
			break
		}
	}
	return expected
}

// expectedWritableBinds is the complete set of read-write host binds: the
// child's private per-execution directories and the one declared shared
// project.
func (f *bwrapFixture) expectedWritableBinds() [][]string {
	return [][]string{
		{f.homePath, "/home/coder"},
		{f.tmpPath, "/tmp"},
		{filepath.Join(f.runtimePath, "xdg"), "/run/user/1000"},
		{f.sharedPath, f.input.SharedChildPath},
	}
}

// requireBwrapMountPolicy pins every host path the sandbox is given. It walks
// the recorded operations, refuses any host-exposing operation other than the
// two bind flags the driver is allowed to use, and then compares the complete
// read-only and read-write bind sets against the exact expected ones. An extra
// mount of any kind, of any host path, fails here.
func requireBwrapMountPolicy(t require.TestingT, f *bwrapFixture, argv bwrapArgv) {
	var readOnly, writable [][]string
	for _, op := range bwrapOps(t, argv) {
		if !bwrapHostExposingFlags[op.flag] {
			continue
		}
		switch op.flag {
		case "--ro-bind":
			readOnly = append(readOnly, op.args)
		case "--bind":
			writable = append(writable, op.args)
		default:
			require.Failf(t, "forbidden host-exposing bubblewrap operation",
				"%s %v must not be used: the driver may only expose host paths with --ro-bind and --bind",
				op.flag, op.args)
		}
	}

	require.ElementsMatch(t, f.expectedReadOnlyBinds(t), readOnly,
		"the read-only host binds must be exactly the allowlist")
	require.Equal(t, f.expectedWritableBinds(), writable,
		"the read-write host binds must be exactly the private directories and the declared shared project")
}

// withOp returns the argument list with an extra operation spliced in ahead of
// the end of options separator, which is where a driver change would add a
// mount.
func (a bwrapArgv) withOp(t *testing.T, op ...string) bwrapArgv {
	t.Helper()

	for i, element := range a {
		if element == "--" {
			mutated := append(bwrapArgv{}, a[:i]...)
			mutated = append(mutated, op...)
			return append(mutated, a[i:]...)
		}
	}
	t.Fatal("the recorded argument list has no end of options separator")
	return nil
}

func TestBwrapDriverSyntax(t *testing.T) {
	t.Parallel()

	output, err := exec.Command("sh", "-n", bwrapDriverScript(t)).CombinedOutput()
	require.NoError(t, err, "sh -n rejected the checked-in driver: %s", output)
}

func TestBwrapDriverRun(t *testing.T) {
	t.Parallel()

	t.Run("ValuesStayOneArgvElement", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		// Each hostile path appears exactly once, as one whole element. A
		// value that had been split, globbed, or expanded would either be
		// missing or show up as several elements.
		require.Equal(t, 1, argv.count(f.sharedPath), "shared host path is not one argv element")
		// The child path appears twice, as the bind destination and as the
		// working directory, and both times as one whole element.
		require.Equal(t, 2, argv.count(f.input.SharedChildPath))
		require.Equal(t, 1, argv.count(f.coderPath))
		require.Equal(t, 1, argv.count(f.tokenPath))
		require.Contains(t, f.sharedPath, "$HOME", "the fixture must carry an unexpanded variable reference")
		require.Contains(t, f.sharedPath, "*", "the fixture must carry a glob character")

		// Nothing in the argument list may look like a shell command line.
		for _, element := range argv {
			require.NotContains(t, element, "\n", "argv element spans lines: %q", element)
		}

		// No command substitution ran: neither the driver's working
		// directory nor the shared project gained the sentinel.
		entries, err := os.ReadDir(f.cwd)
		require.NoError(t, err)
		require.Empty(t, entries, "the driver's working directory must stay empty")
		_, err = os.Stat(filepath.Join(f.sharedPath, bwrapSentinelName))
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("NetworkIsPreserved", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		// Network egress isolation is out of scope: the child agent has to
		// reach the deployment, so the network namespace is shared.
		require.False(t, argv.has("--unshare-net"), "the driver must not unshare the network namespace")
		require.False(t, argv.has("--unshare-all"))
	})

	t.Run("NamespacesAndPrivilegesAreConstrained", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		for _, flag := range []string{
			"--unshare-user",
			"--unshare-pid",
			"--unshare-ipc",
			"--unshare-uts",
			"--unshare-cgroup-try",
			"--die-with-parent",
			"--new-session",
			"--clearenv",
		} {
			require.True(t, argv.has(flag), "%s is missing", flag)
		}
		require.Equal(t, "ALL", argv.value(t, "--cap-drop"))
		require.Equal(t, "1000", argv.value(t, "--uid"))
		require.Equal(t, "1000", argv.value(t, "--gid"))

		// A fresh /proc and bubblewrap's minimal /dev, neither bound from
		// the host.
		require.Equal(t, "/proc", argv.value(t, "--proc"))
		require.Equal(t, "/dev", argv.value(t, "--dev"))
	})

	t.Run("NoHostFilesystemIsExposed", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		binds := argv.hostBinds(append(append([]string{},
			bwrapReadWriteBindFlags...), bwrapReadOnlyBindFlags...)...)
		require.NotEmpty(t, binds)

		// The private empty root is bubblewrap's own tmpfs, so the host root
		// and the directories that would carry the host's libraries and
		// interpreters are never bound.
		forbidden := []string{
			"/", "/usr", "/lib", "/lib64", "/bin", "/sbin", "/opt", "/var", "/root",
			f.parentHome,
			f.stateRoot,
			f.statePath,
			f.dockerSocket,
		}
		for _, bind := range binds {
			source := bind[0]
			for _, path := range forbidden {
				require.NotEqual(t, path, source, "host path %s must not be bound into the sandbox", path)
			}
			require.False(t, strings.HasPrefix(source, f.parentHome+string(os.PathSeparator)),
				"a parent home path must not be bound: %s", source)
		}
		// The Docker socket and the parent's dotfile are not mentioned at
		// all, in any position.
		require.Equal(t, 0, argv.count(f.dockerSocket))
		require.Equal(t, 0, argv.count(filepath.Join(f.parentHome, ".netrc")))
		require.Equal(t, 0, argv.count(f.parentHome))
		require.Equal(t, 0, argv.count(f.stateRoot))
		require.Equal(t, 0, argv.count(f.statePath))
	})

	t.Run("HostMountsAreExactlyTheAllowlist", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		// Every host path the sandbox is given, read-only and read-write,
		// against the exact expected set. Nothing else is exposed, and the
		// only writable host directory is the declared shared project.
		requireBwrapMountPolicy(t, f, argv)

		// The private directories are separate paths, none of them inside
		// the shared project.
		for _, private := range []string{f.homePath, f.tmpPath, f.runtimePath} {
			require.False(t, strings.HasPrefix(private, f.sharedPath))
		}
	})

	t.Run("CoderBinaryAndTokenAreReadOnly", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		readOnly := argv.hostBinds(bwrapReadOnlyBindFlags...)
		require.Contains(t, readOnly, []string{f.coderPath, "/opt/coder/coder"})
		require.Contains(t, readOnly, []string{f.tokenPath, "/run/coder/token"})

		// Neither is also bound read-write.
		writable := argv.hostBinds(bwrapReadWriteBindFlags...)
		for _, bind := range writable {
			require.NotEqual(t, f.coderPath, bind[0])
			require.NotEqual(t, f.tokenPath, bind[0])
		}

		// The token's value never reaches the argument list, and the driver
		// never prints it.
		for _, element := range argv {
			require.NotContains(t, element, testAuthToken)
		}
	})

	t.Run("MinimalRootLayout", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		// /bin is the static BusyBox binary plus one symlink per applet,
		// created inside the sandbox rather than bound from a host
		// directory.
		require.Contains(t, argv.hostBinds(bwrapReadOnlyBindFlags...),
			[]string{filepath.Join(f.binDir, "busybox"), "/bin/busybox"})
		symlinks := argv.pairs("--symlink")
		require.NotEmpty(t, symlinks)
		applets := map[string]bool{}
		for _, link := range symlinks {
			require.Equal(t, "/bin/busybox", link[0])
			require.True(t, strings.HasPrefix(link[1], "/bin/"))
			applets[strings.TrimPrefix(link[1], "/bin/")] = true
		}
		// Enough for a shell session, a terminal, and the example HTTP
		// server the reference template runs.
		for _, applet := range []string{"sh", "env", "ls", "cat", "ps", "stty", "tty", "httpd"} {
			require.True(t, applets[applet], "the %s applet is missing", applet)
		}

		// The generated account files are the driver's own, written under
		// the private runtime directory and mounted read-only.
		generated := filepath.Join(f.runtimePath, "etc")
		readOnly := argv.hostBinds(bwrapReadOnlyBindFlags...)
		for name, dest := range map[string]string{
			"passwd":        "/etc/passwd",
			"group":         "/etc/group",
			"nsswitch.conf": "/etc/nsswitch.conf",
			"os-release":    "/etc/os-release",
		} {
			require.Contains(t, readOnly, []string{filepath.Join(generated, name), dest})
			content, err := os.ReadFile(filepath.Join(generated, name))
			require.NoError(t, err)
			require.NotEmpty(t, content)
		}
		passwd, err := os.ReadFile(filepath.Join(generated, "passwd"))
		require.NoError(t, err)
		require.Contains(t, string(passwd), "coder:x:1000:1000:")
		require.Contains(t, string(passwd), ":/home/coder:/bin/sh")

		osRelease, err := os.ReadFile(filepath.Join(generated, "os-release"))
		require.NoError(t, err)
		require.Contains(t, string(osRelease), "ID=coder-sandbox\n")
		require.Contains(t, string(osRelease), "ID_LIKE=debian\n")
		require.Contains(t, string(osRelease), "VERSION_ID=\"1\"\n")

		// The host's optional resolver and trust files are covered by the
		// exact read-only allowlist in HostMountsAreExactlyTheAllowlist,
		// which requires each of them to be bound exactly when this machine
		// has it.
	})

	t.Run("ChildCommandAndEnvironment", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		f.requireRun(t, OperationRun, f.runInput)
		argv := f.argv(t)

		require.Equal(t, f.input.SharedChildPath, argv.value(t, "--chdir"))

		// The child reads its token from the fixed path the driver bound it
		// at, never from an argument or an inherited variable.
		require.Equal(t, f.input.CoderURL, argv.setenv(t, "CODER_AGENT_URL"))
		require.Equal(t, "token", argv.setenv(t, "CODER_AGENT_AUTH"))
		require.Equal(t, "/run/coder/token", argv.setenv(t, "CODER_AGENT_TOKEN_FILE"))
		require.Equal(t, "/home/coder", argv.setenv(t, "HOME"))
		require.Equal(t, "/tmp", argv.setenv(t, "TMPDIR"))
		require.Equal(t, "/run/user/1000", argv.setenv(t, "XDG_RUNTIME_DIR"))
		require.Equal(t, "/opt/coder:/bin", argv.setenv(t, "PATH"))
		for _, pair := range argv.pairs("--setenv") {
			require.NotEqual(t, "CODER_AGENT_TOKEN", pair[0], "the token must never be an environment variable")
			require.NotContains(t, pair[1], testAuthToken)
		}

		require.Equal(t, []string{
			"/opt/coder/coder", "agent",
			"--log-dir", "/home/coder/.coder-agent/log",
			"--script-data-dir", "/home/coder/.coder-agent/scripts",
			"--subagent-exec-state-dir", "/home/coder/.coder-agent/subagent-exec",
			"--pprof-address=",
			"--prometheus-address=",
			"--debug-address=",
			"--devcontainers-enable=false",
		}, argv.command(t))

		// The state directories the child agent writes to exist already and
		// are private to this execution.
		for _, dir := range []string{"log", "scripts", "subagent-exec"} {
			info, err := os.Stat(filepath.Join(f.homePath, ".coder-agent", dir))
			require.NoError(t, err)
			require.True(t, info.IsDir())
		}
	})
}

// TestBwrapDriverMountPolicyRejectsExtraMounts shows that the mount policy
// assertion is the thing catching an extra host mount rather than passing by
// construction. Each case takes the argument list the checked-in driver really
// produced, splices one extra operation into it, and requires the assertion to
// reject the result. Only the recorded argument list is mutated: the driver
// script is never modified.
func TestBwrapDriverMountPolicyRejectsExtraMounts(t *testing.T) {
	t.Parallel()

	f := newBwrapFixture(t)
	f.requireRun(t, OperationRun, f.runInput)
	argv := f.argv(t)

	// The unmutated argument list satisfies the policy, so every failure
	// below is caused by the mutation and nothing else.
	require.Empty(t, bwrapPolicyFailures(f, argv),
		"the checked-in driver's own argument list must satisfy the mount policy")

	for name, mutation := range map[string][]string{
		"ShadowFile":       {"--ro-bind", "/etc/shadow", "/etc/shadow"},
		"ArbitraryPath":    {"--ro-bind", "/etc/machine-id", "/etc/machine-id"},
		"HostUsr":          {"--ro-bind", "/usr", "/usr"},
		"HostLib":          {"--ro-bind", "/lib", "/lib"},
		"HostRoot":         {"--ro-bind", "/", "/host"},
		"ParentHome":       {"--ro-bind", f.parentHome, "/home/coder/host"},
		"LauncherState":    {"--bind", f.stateRoot, "/mnt/state"},
		"DockerSocket":     {"--bind", f.dockerSocket, "/run/docker.sock"},
		"DevBind":          {"--dev-bind", "/dev/sda", "/dev/sda"},
		"ReadOnlyBindTry":  {"--ro-bind-try", "/etc/shadow", "/etc/shadow"},
		"ReadWriteBindTry": {"--bind-try", "/root", "/root"},
		"OverlaySource":    {"--overlay-src", "/usr"},
		"UnknownFlag":      {"--smuggle-host", "/usr"},
		// A duplicate of an allowed mount is still an extra mount: the
		// comparison is a multiset, so the second copy has nothing to match.
		"DuplicateToken": {"--ro-bind", f.tokenPath, "/run/coder/token"},
		// Rebinding an allowed read-only source read-write must not pass as
		// the read-only entry it resembles.
		"WritableCoderBinary": {"--bind", f.coderPath, "/opt/coder/coder"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			failures := bwrapPolicyFailures(f, argv.withOp(t, mutation...))
			require.NotEmpty(t, failures,
				"the mount policy assertion accepted the extra operation %v", mutation)
		})
	}
}

// TestBwrapDriverRealJQ runs the driver against the real jq and the real
// protocol document the launcher writes, so the filters the driver uses are
// exercised rather than the fake that answers from fixtures. It is gated on jq
// being installed where the launcher's controlled PATH can find it, because
// the rest of these tests deliberately need no host tooling at all.
func TestBwrapDriverRealJQ(t *testing.T) {
	t.Parallel()

	f := newBwrapFixture(t)
	jqPath, err := exec.LookPath("jq")
	if err != nil {
		t.Skipf("jq is not installed: %v", err)
	}
	if !strings.Contains(DefaultPath, filepath.Dir(jqPath)) {
		t.Skipf("jq at %s is outside the launcher's controlled PATH", jqPath)
	}
	// Dropping the fake lets the controlled PATH resolve the real jq.
	require.NoError(t, os.Remove(filepath.Join(f.binDir, "jq")))

	f.requireRun(t, OperationRun, f.runInput)
	argv := f.argv(t)

	// Real jq hands back the hostile shared path byte for byte, as one argv
	// element, and the rest of the document lands where it should.
	require.Equal(t, 1, argv.count(f.sharedPath))
	require.Contains(t, argv.hostBinds(bwrapReadWriteBindFlags...),
		[]string{f.sharedPath, f.input.SharedChildPath})
	require.Contains(t, argv.hostBinds(bwrapReadOnlyBindFlags...),
		[]string{f.tokenPath, "/run/coder/token"})
	require.Equal(t, f.input.CoderURL, argv.setenv(t, "CODER_AGENT_URL"))
	require.Equal(t, f.input.SharedChildPath, argv.value(t, "--chdir"))

	// The protocol version check reads a JSON number, not a string.
	require.NoError(t, os.Remove(f.argvFile))
	document, err := os.ReadFile(f.runInput)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.runInput,
		[]byte(strings.Replace(string(document), `"protocol_version":1`, `"protocol_version":"1"`, 1)),
		privateFileMode))

	output, err := f.run(t, OperationRun, f.runInput)
	require.Error(t, err)
	require.Contains(t, output, "unsupported driver protocol")
	require.NoFileExists(t, f.argvFile)
}

func TestBwrapDriverRejects(t *testing.T) {
	t.Parallel()

	t.Run("WrongArgumentCount", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		for _, args := range [][]string{{}, {string(OperationRun)}, {string(OperationRun), f.runInput, "extra"}} {
			cmd := exec.Command(f.script, args...)
			cmd.Dir = f.cwd
			cmd.Env = []string{"PATH=" + f.binDir + string(os.PathListSeparator) + DefaultPath}
			output, err := cmd.CombinedOutput()
			require.Error(t, err, "argument list %v was accepted: %s", args, output)
		}
		require.NoFileExists(t, f.argvFile, "bwrap must not run for a rejected invocation")
	})

	t.Run("UnknownOperation", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		output, err := f.run(t, "sabotage", f.runInput)
		require.Error(t, err)
		require.Contains(t, output, "unsupported operation")
		require.NoFileExists(t, f.argvFile)
	})

	t.Run("OperationMismatch", func(t *testing.T) {
		t.Parallel()

		// The argument list says run, the document says cleanup.
		f := newBwrapFixture(t)
		output, err := f.run(t, OperationRun, f.cleanupInput)
		require.Error(t, err)
		require.Contains(t, output, "does not match")
		require.NoFileExists(t, f.argvFile)
	})

	t.Run("UnsupportedProtocolVersion", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		fixture := filepath.Join(f.fixtureDir, string(OperationRun), "numbers", "protocol_version")
		require.NoError(t, os.WriteFile(fixture, []byte("2"), 0o600))

		output, err := f.run(t, OperationRun, f.runInput)
		require.Error(t, err)
		require.Contains(t, output, "unsupported driver protocol")
		require.NoFileExists(t, f.argvFile)
	})

	t.Run("MissingRequiredField", func(t *testing.T) {
		t.Parallel()

		for _, field := range []string{"coder_url", "coder_binary_path", "token_file_path", "shared_host_path", "shared_child_path", "runtime_path"} {
			f := newBwrapFixture(t)
			fixture := filepath.Join(f.fixtureDir, string(OperationRun), "strings", field)
			require.NoError(t, os.WriteFile(fixture, nil, 0o600))

			output, err := f.run(t, OperationRun, f.runInput)
			require.Error(t, err, "an empty %s was accepted", field)
			require.Contains(t, output, field)
			require.NoFileExists(t, f.argvFile)
		}
	})

	t.Run("RelativePath", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		fixture := filepath.Join(f.fixtureDir, string(OperationRun), "strings", "shared_child_path")
		require.NoError(t, os.WriteFile(fixture, []byte("workspace/project"), 0o600))

		output, err := f.run(t, OperationRun, f.runInput)
		require.Error(t, err)
		require.Contains(t, output, "must be an absolute path")
		require.NoFileExists(t, f.argvFile)
	})

	t.Run("FilesystemRoot", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		fixture := filepath.Join(f.fixtureDir, string(OperationRun), "strings", "runtime_path")
		require.NoError(t, os.WriteFile(fixture, []byte("/"), 0o600))

		output, err := f.run(t, OperationRun, f.runInput)
		require.Error(t, err)
		require.Contains(t, output, "must not be the filesystem root")
		require.NoFileExists(t, f.argvFile)
	})

	t.Run("MissingTool", func(t *testing.T) {
		t.Parallel()

		f := newBwrapFixture(t)
		require.NoError(t, os.Remove(filepath.Join(f.binDir, "busybox")))

		output, err := f.run(t, OperationRun, f.runInput)
		require.Error(t, err)
		require.Contains(t, output, "busybox")
		require.NoFileExists(t, f.argvFile)
	})
}

func TestBwrapDriverCleanup(t *testing.T) {
	t.Parallel()

	f := newBwrapFixture(t)
	f.requireRun(t, OperationRun, f.runInput)
	// The run's recording is discarded, so a bubblewrap invocation from
	// cleanup would be visible as the file coming back.
	require.NoError(t, os.Remove(f.argvFile))

	// Content that cleanup must leave alone: the owner's shared project, the
	// child's private home and runtime content, and the launcher's token
	// file, which the launcher itself reclaims after cleanup returns.
	sharedFile := filepath.Join(f.sharedPath, "owner-file")
	require.NoError(t, os.WriteFile(sharedFile, []byte("owner content"), 0o600))
	homeFile := filepath.Join(f.homePath, "child-file")
	require.NoError(t, os.WriteFile(homeFile, []byte("child content"), 0o600))
	runtimeFile := filepath.Join(f.runtimePath, "xdg", "child-socket-marker")
	require.NoError(t, os.WriteFile(runtimeFile, []byte("child content"), 0o600))

	generated := filepath.Join(f.runtimePath, "etc")
	require.DirExists(t, generated)

	// Cleanup is idempotent: it succeeds whether or not the run left
	// anything behind, and there are no namespaces or mounts to tear down
	// because bubblewrap scopes both to the sandbox process.
	for attempt := 0; attempt < 3; attempt++ {
		f.requireRun(t, OperationCleanup, f.cleanupInput)

		require.NoDirExists(t, generated, "the generated support files must be gone")
		require.FileExists(t, sharedFile)
		require.FileExists(t, homeFile)
		require.FileExists(t, runtimeFile)
		require.FileExists(t, f.tokenPath)
		require.DirExists(t, f.sharedPath)

		content, err := os.ReadFile(sharedFile)
		require.NoError(t, err)
		require.Equal(t, "owner content", string(content))
	}

	// Cleanup never invokes bubblewrap: the sandbox is already gone.
	require.NoFileExists(t, f.argvFile)
}
