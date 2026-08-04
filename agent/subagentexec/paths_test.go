package subagentexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// requireDir creates dir and returns it, so a fixture reads as the layout
// it builds.
func requireDir(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return dir
}

// pathDeclaration is the reference declaration: the shared child path is the
// one the reference template uses, and the shared host path is supplied per
// case.
func pathDeclaration(sharedHostPath string) agentsdk.SubagentExecution {
	decl := testDeclaration()
	decl.SharedHostPath = sharedHostPath
	decl.SharedChildPath = "/workspace/project"
	return decl
}

// TestValidateDeclaredPaths_ValidHostPaths covers the two shapes a
// deployment may declare: the project root itself, and a directory inside
// it. Both are returned in canonical form, which is what the driver mounts.
func TestValidateDeclaredPaths_ValidHostPaths(t *testing.T) {
	t.Parallel()

	paths := newTestPaths(t)
	nested := requireDir(t, filepath.Join(paths.project, "nested", "shared"))

	// A symlink outside the project that points inside it is accepted, and
	// resolves to the path it really names.
	link := filepath.Join(paths.home, "shared-link")
	require.NoError(t, os.Symlink(nested, link))

	for _, tc := range []struct {
		name     string
		declared string
		want     string
	}{
		{name: "ProjectRoot", declared: paths.project, want: paths.project},
		{name: "Descendant", declared: nested, want: nested},
		{name: "TrailingSeparator", declared: paths.project + string(filepath.Separator), want: paths.project},
		{name: "SymlinkIntoProject", declared: link, want: nested},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			canonical, err := validateDeclaredPaths(paths.context(), paths.state, pathDeclaration(tc.declared))
			require.NoError(t, err)
			require.Equal(t, tc.want, canonical)
		})
	}
}

// TestValidateDeclaredPaths_RejectsHostPaths covers every host-path rule.
// Each case asserts the generic reason that is reported, which is the only
// path-policy text that leaves the agent.
func TestValidateDeclaredPaths_RejectsHostPaths(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// setup returns the path context, the state root, and the declared
		// shared host path.
		setup  func(t *testing.T) (PathContext, string, string)
		reason string
	}{
		{
			name: "OutsideProjectRoot",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				outside := requireDir(t, filepath.Join(paths.home, "private"))
				return paths.context(), paths.state, outside
			},
			reason: reasonSharedHostOutsideProject,
		},
		{
			name: "SymlinkEscapesProjectRoot",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				outside := requireDir(t, filepath.Join(paths.home, "private"))
				// A lexical comparison would see a path inside the project.
				escape := filepath.Join(paths.project, "escape")
				require.NoError(t, os.Symlink(outside, escape))
				return paths.context(), paths.state, escape
			},
			reason: reasonSharedHostOutsideProject,
		},
		{
			name: "Relative",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				return paths.context(), paths.state, "project"
			},
			reason: reasonSharedHostUnresolvable,
		},
		{
			name: "Missing",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				return paths.context(), paths.state, filepath.Join(paths.project, "missing")
			},
			reason: reasonSharedHostUnresolvable,
		},
		{
			name: "NotADirectory",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				file := filepath.Join(paths.project, "file")
				require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
				return paths.context(), paths.state, file
			},
			reason: reasonSharedHostUnresolvable,
		},
		{
			name: "Empty",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				return paths.context(), paths.state, ""
			},
			reason: reasonSharedHostUnresolvable,
		},
		{
			name: "ProjectRootIsFilesystemRoot",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				context := paths.context()
				// A project root of / would make containment meaningless.
				context.ProjectRoot = string(filepath.Separator)
				return context, paths.state, paths.project
			},
			reason: reasonProjectRootIsRoot,
		},
		{
			name: "ProjectRootUnknown",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				context := paths.context()
				context.ProjectRoot = ""
				return context, paths.state, paths.project
			},
			reason: reasonProjectRootUnavailable,
		},
		{
			name: "ProjectRootMissing",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				context := paths.context()
				context.ProjectRoot = filepath.Join(paths.home, "missing")
				return context, paths.state, paths.project
			},
			reason: reasonProjectRootUnavailable,
		},
		{
			name: "SharedPathIsParentHome",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				// The owner declared their whole home as the project.
				return PathContext{ProjectRoot: paths.home, ParentHome: paths.home}, paths.state, paths.home
			},
			reason: reasonSharedHostIsParentHome,
		},
		{
			name: "SharedPathIsInsideParentSSH",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				shared := requireDir(t, filepath.Join(paths.home, ".ssh", "shared"))
				return PathContext{ProjectRoot: paths.home, ParentHome: paths.home}, paths.state, shared
			},
			reason: reasonSharedHostOverlapsSSH,
		},
		{
			name: "SharedPathContainsParentSSHTarget",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				// The owner's SSH directory is a symlink into the project,
				// so a shared project directory contains their keys.
				keys := requireDir(t, filepath.Join(paths.project, "dotfiles", "ssh"))
				require.NoError(t, os.Symlink(keys, filepath.Join(paths.home, ".ssh")))
				return paths.context(), paths.state, paths.project
			},
			reason: reasonSharedHostOverlapsSSH,
		},
		{
			name: "ParentHomeMissing",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				context := paths.context()
				context.ParentHome = filepath.Join(paths.home, "missing")
				return context, paths.state, paths.project
			},
			reason: reasonParentHomeUnavailable,
		},
		{
			name: "SharedPathContainsPrivateState",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				// The state root does not exist yet, so it is judged where
				// it would be created.
				return paths.context(), filepath.Join(paths.project, "state"), paths.project
			},
			reason: reasonSharedHostOverlapsState,
		},
		{
			name: "SharedPathIsInsidePrivateState",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				shared := requireDir(t, filepath.Join(paths.project, "state", "shared"))
				return paths.context(), filepath.Join(paths.project, "state"), shared
			},
			reason: reasonSharedHostOverlapsState,
		},
		{
			name: "PrivateStateReachedThroughSymlink",
			setup: func(t *testing.T) (PathContext, string, string) {
				paths := newTestPaths(t)
				// An ancestor of the state root is a symlink into the
				// project, so private state would land in the shared tree.
				link := filepath.Join(paths.home, "link")
				require.NoError(t, os.Symlink(paths.project, link))
				return paths.context(), filepath.Join(link, "state"), paths.project
			},
			reason: reasonSharedHostOverlapsState,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			context, stateRoot, declared := tc.setup(t)
			canonical, err := validateDeclaredPaths(context, stateRoot, pathDeclaration(declared))
			require.Empty(t, canonical)
			require.EqualError(t, err, "shared project path policy: "+tc.reason)
			// The reported error carries the rule only. The paths involved
			// stay in the local diagnostic.
			if filepath.IsAbs(declared) {
				require.NotContains(t, err.Error(), declared)
			}
			require.NotEmpty(t, policyDetail(err))
		})
	}
}

// TestValidateDeclaredPaths_UnknownParentHomeSkipsHomeRules documents the
// limitation of an agent whose home directory cannot be determined: the
// home-root and SSH rules cannot be enforced, so they are skipped rather
// than guessed from the project directory. Containment in the project root
// still applies.
func TestValidateDeclaredPaths_UnknownParentHomeSkipsHomeRules(t *testing.T) {
	t.Parallel()

	paths := newTestPaths(t)
	shared := requireDir(t, filepath.Join(paths.home, ".ssh", "shared"))
	context := PathContext{ProjectRoot: paths.home}

	canonical, err := validateDeclaredPaths(context, paths.state, pathDeclaration(shared))
	require.NoError(t, err)
	require.Equal(t, shared, canonical)

	// With the home directory known, the same declaration is rejected.
	context.ParentHome = paths.home
	_, err = validateDeclaredPaths(context, paths.state, pathDeclaration(shared))
	require.EqualError(t, err, "shared project path policy: "+reasonSharedHostOverlapsSSH)
}

// TestValidateDeclaredPaths_UnknownStateRootSkipsStateRule pins the
// behavior of a manager without a configured state root: there is no
// private state to protect, so the overlap rule has nothing to compare.
func TestValidateDeclaredPaths_UnknownStateRootSkipsStateRule(t *testing.T) {
	t.Parallel()

	paths := newTestPaths(t)
	canonical, err := validateDeclaredPaths(paths.context(), "", pathDeclaration(paths.project))
	require.NoError(t, err)
	require.Equal(t, paths.project, canonical)
}

func TestValidateDeclaredPaths_ChildPaths(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		child string
		// reason is empty for the paths the policy accepts.
		reason string
	}{
		{name: "Reference", child: "/workspace/project"},
		{name: "SingleComponent", child: "/workspace"},
		{name: "UnrelatedRoot", child: "/srv/project"},
		{name: "Empty", child: "", reason: reasonChildPathInvalid},
		{name: "Relative", child: "workspace/project", reason: reasonChildPathInvalid},
		{name: "RelativeDot", child: "./workspace", reason: reasonChildPathInvalid},
		{name: "Traversal", child: "/workspace/../srv", reason: reasonChildPathInvalid},
		{name: "TraversalAboveRoot", child: "/../workspace", reason: reasonChildPathInvalid},
		{name: "CurrentDirectoryComponent", child: "/workspace/./project", reason: reasonChildPathInvalid},
		{name: "LeadingDuplicateSeparator", child: "//workspace/project", reason: reasonChildPathInvalid},
		{name: "InnerDuplicateSeparator", child: "/workspace//project", reason: reasonChildPathInvalid},
		{name: "TrailingSeparator", child: "/workspace/project/", reason: reasonChildPathInvalid},
		{name: "NUL", child: "/workspace/pro\x00ject", reason: reasonChildPathInvalid},
		{name: "Newline", child: "/workspace/pro\nject", reason: reasonChildPathInvalid},
		{name: "Root", child: "/", reason: reasonChildPathIsRoot},
		{name: "ReservedEtc", child: "/etc", reason: reasonChildPathReserved},
		{name: "ReservedInsideEtc", child: "/etc/project", reason: reasonChildPathReserved},
		{name: "ReservedHome", child: "/home/coder/project", reason: reasonChildPathReserved},
		{name: "ReservedTmp", child: "/tmp/project", reason: reasonChildPathReserved},
		{name: "ReservedRun", child: "/run/user/1000", reason: reasonChildPathReserved},
		{name: "ReservedProc", child: "/proc", reason: reasonChildPathReserved},
		{name: "ReservedDev", child: "/dev/shm", reason: reasonChildPathReserved},
		{name: "ReservedBin", child: "/bin", reason: reasonChildPathReserved},
		{name: "ReservedCoder", child: "/opt/coder/project", reason: reasonChildPathReserved},
		// A child path that contains a reserved path is rejected too: the
		// sandbox would have to mount the shared project over it.
		{name: "ContainsReservedCoder", child: "/opt", reason: reasonChildPathReserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			paths := newTestPaths(t)
			decl := pathDeclaration(paths.project)
			decl.SharedChildPath = tc.child

			canonical, err := validateDeclaredPaths(paths.context(), paths.state, decl)
			if tc.reason == "" {
				require.NoError(t, err)
				require.Equal(t, paths.project, canonical)
				return
			}
			require.EqualError(t, err, "shared project path policy: "+tc.reason)
			require.Empty(t, canonical)
		})
	}
}

// TestManager_PassesCanonicalSharedPathToDriver proves the driver receives
// the resolved path rather than the declared one, so it mounts exactly what
// the policy judged.
func TestManager_PassesCanonicalSharedPathToDriver(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	m := newManager(t, driver)

	nested := requireDir(t, filepath.Join(m.paths.project, "nested"))
	link := filepath.Join(m.paths.home, "shared-link")
	require.NoError(t, os.Symlink(nested, link))

	decl := m.declaration()
	decl.SharedHostPath = link
	m.reconcile(controller, decl)

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, nested, launch.SharedHostPath)
	// The declaration itself is handed over unchanged, so a driver can
	// still see what the deployment asked for.
	require.Equal(t, link, launch.Declaration.SharedHostPath)
}

// TestManager_RejectedPathsReportFailedAndRetry covers the whole failure
// contract: the rejection is reported as FAILED under the version the
// launch acquired, no driver runs, no private state is created, and the
// declaration is retried unchanged once the filesystem satisfies the
// policy.
func TestManager_RejectedPathsReportFailedAndRetry(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	controller.setAcquireResponseFn(func(*proto.AcquireSubagentExecutionRequest) *proto.AcquireSubagentExecutionResponse {
		childAgentID := uuid.New()
		return &proto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          testAuthToken,
			AcquisitionVersion: 11,
		}
	})
	driver := newFakeDriver()
	m := newManager(t, driver)

	// The declared shared path does not exist yet, which is the case a
	// deployment fixes by creating it.
	shared := filepath.Join(m.paths.project, "shared")
	decl := m.declaration()
	decl.SharedHostPath = shared
	m.reconcile(controller, decl)

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
	require.EqualValues(t, 11, report.GetAcquisitionVersion())
	require.Equal(t, decl.ExecutionID[:], report.GetExecutionId())
	require.Contains(t, report.GetError(), "shared project path policy")
	// Nothing about the host filesystem or the launcher's private state is
	// reported.
	require.NotContains(t, report.GetError(), shared)
	require.NotContains(t, report.GetError(), m.paths.state)
	require.NotContains(t, report.GetError(), testAuthToken)

	require.Zero(t, driver.startCount())
	// No token file, protocol document, or state directory was created: the
	// rejection happens before the driver is invoked.
	require.NoDirExists(t, m.paths.state)

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateFailed, statuses[0].State)
	require.EqualValues(t, 11, statuses[0].AcquisitionVersion)
	require.True(t, strings.HasPrefix(statuses[0].LastError, "shared project path policy"))

	// The same declaration launches once the path exists, which is what
	// makes a rejection retryable rather than terminal.
	requireDir(t, shared)
	m.reconcile(controller, decl)

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, decl.ExecutionID, launch.Declaration.ExecutionID)
	require.Equal(t, shared, launch.SharedHostPath)
	require.Equal(t, 2, controller.acquisitionCountFor(decl.ExecutionID))
}

// TestManager_RejectedChildPathNeverAcquiresTwice pins that a rejected
// child path still costs exactly one acquisition per reconcile: the policy
// runs after Acquire so the failure can be reported, not before.
func TestManager_RejectedChildPathNeverLaunches(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := m.declaration()
	decl.SharedChildPath = "/etc"
	m.reconcile(controller, decl)

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
	require.Contains(t, report.GetError(), reasonChildPathReserved)
	require.Equal(t, 1, controller.acquisitionCountFor(decl.ExecutionID))
	require.Zero(t, driver.startCount())
}
