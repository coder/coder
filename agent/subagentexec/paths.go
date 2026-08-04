package subagentexec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// PathContext is the parent-side path context one reconciliation judges
// declared shared project paths against. It travels with the manifest that
// carried the declarations, so a manifest that moves the project directory
// is judged against the directory that manifest named.
type PathContext struct {
	// ProjectRoot is the parent agent's expanded working directory: the
	// root of the project the human owner may share with a child. An empty
	// ProjectRoot rejects every declaration, because there is then nothing
	// to bound the declared shared path against.
	ProjectRoot string
	// ParentHome is the parent agent's home directory. It keeps a
	// declaration from sharing the owner's home root or SSH directory. It
	// is empty only when the launcher could not determine the home
	// directory at all, which skips exactly those two rules; the remaining
	// rules, including containment in the project root, still apply.
	//
	// The launcher never guesses a home directory from the project root: a
	// project under /srv or /workspace says nothing about where the owner's
	// home is, and a wrong guess would either reject a legitimate project
	// or accept a shared path over the real home.
	ParentHome string
}

// reservedChildPaths are the paths a declared child path may neither be,
// contain, nor live inside. They are the directories the sandbox itself
// owns: mapping the shared project over them would either break the child's
// own filesystem or make the parent's tree reachable through a path the
// child expects to be private.
var reservedChildPaths = []string{
	"/bin",
	"/dev",
	"/etc",
	"/home",
	"/opt/coder",
	"/proc",
	"/run",
	"/tmp",
}

// The reasons a declaration is rejected for. They are the only path-policy
// text that reaches coderd, so each one is a short, fixed string that
// describes the rule rather than the filesystem: a workspace owner can read
// a reported error, and the launcher's private state layout, the parent's
// resolved home, and every canonicalized host path stay local.
const (
	reasonProjectRootUnavailable = "the agent project directory is not available"
	reasonProjectRootIsRoot      = "the agent project directory must not be the filesystem root"
	reasonParentHomeUnavailable  = "the parent home directory is not available"
	reasonStateRootUnavailable   = "the launcher private state directory is not available"

	reasonSharedHostUnresolvable   = "the declared shared host path is not an existing directory"
	reasonSharedHostOutsideProject = "the declared shared host path is outside the agent project directory"
	reasonSharedHostIsParentHome   = "the declared shared host path must not be the parent home directory"
	reasonSharedHostOverlapsSSH    = "the declared shared host path overlaps the parent SSH directory"
	reasonSharedHostOverlapsState  = "the declared shared host path overlaps the launcher private state directory"

	reasonChildPathInvalid  = "the declared shared child path must be an absolute, lexically clean path"
	reasonChildPathIsRoot   = "the declared shared child path must not be the filesystem root"
	reasonChildPathReserved = "the declared shared child path overlaps a reserved directory in the child"
)

// pathPolicyError is a rejected declaration. Error carries one of the fixed
// reasons above and is what the manager reports; detail carries the same
// rejection with the paths involved and is written to the parent agent's
// log only.
type pathPolicyError struct {
	reason string
	detail string
}

var _ error = (*pathPolicyError)(nil)

func (e *pathPolicyError) Error() string {
	return "shared project path policy: " + e.reason
}

// Detail is the local-only diagnostic. It is deliberately not part of
// Error, and pathPolicyError deliberately does not unwrap to it, so a
// report cannot pick it up through error formatting.
func (e *pathPolicyError) Detail() string {
	return e.detail
}

func policyError(reason, detailFormat string, args ...any) error {
	return &pathPolicyError{reason: reason, detail: fmt.Sprintf(detailFormat, args...)}
}

// policyDetail returns the local-only diagnostic of a path-policy
// rejection, and the error's own message for anything else.
func policyDetail(err error) string {
	var policyErr *pathPolicyError
	if xerrors.As(err, &policyErr) {
		return policyErr.Detail()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// validateDeclaredPaths enforces the shared-project path contract for one
// declaration and returns the canonical shared host path the driver must
// use. It runs after the child's credentials are acquired and before the
// driver is invoked, so a rejection is reported under the acquired version
// and no token file is ever written for it.
//
// The contract is about paths only. Whatever the owner places inside the
// declared project directory is shared with the child by design, so the
// launcher does not scan the project for agent sockets, keys, or other
// sensitive content. Such a scan would be a race the child could win, and
// it would suggest that a shared directory is filtered when it is not.
// Keeping something away from the child means keeping it out of the
// declared project directory.
func validateDeclaredPaths(paths PathContext, stateRoot string, decl agentsdk.SubagentExecution) (string, error) {
	// The child path is judged first: it is a pure lexical check that needs
	// no filesystem access.
	if err := validateChildPath(decl.SharedChildPath); err != nil {
		return "", err
	}
	return validateHostPath(paths, stateRoot, decl.SharedHostPath)
}

// validateHostPath resolves the declared shared host path and judges it
// against the parent's project directory, home directory, and the
// launcher's private state root. Every comparison is between canonical
// paths, so a symlink cannot make a shared path look like it is inside the
// project when it really escapes it.
func validateHostPath(paths PathContext, stateRoot, declared string) (string, error) {
	projectRoot, err := canonicalDir(paths.ProjectRoot)
	if err != nil {
		return "", policyError(reasonProjectRootUnavailable,
			"resolve agent project directory: %v", err)
	}
	if filepath.Dir(projectRoot) == projectRoot {
		// A project root of / would make every host path a descendant, so
		// the containment rule below would permit anything.
		return "", policyError(reasonProjectRootIsRoot,
			"agent project directory %s is the filesystem root", projectRoot)
	}

	shared, err := canonicalDir(declared)
	if err != nil {
		return "", policyError(reasonSharedHostUnresolvable,
			"resolve declared shared host path: %v", err)
	}
	if !isWithin(shared, projectRoot) {
		return "", policyError(reasonSharedHostOutsideProject,
			"declared shared host path %s is outside the agent project directory %s", shared, projectRoot)
	}

	if paths.ParentHome != "" {
		home, err := canonicalDir(paths.ParentHome)
		if err != nil {
			// The home directory was named but cannot be resolved, so the
			// two rules below cannot be enforced. Guessing would be worse
			// than refusing: the declaration is retried once the home
			// directory resolves.
			return "", policyError(reasonParentHomeUnavailable,
				"resolve parent home directory: %v", err)
		}
		if shared == home {
			return "", policyError(reasonSharedHostIsParentHome,
				"declared shared host path %s is the parent home directory", shared)
		}
		for _, sshPath := range parentSSHPaths(home) {
			if overlaps(shared, sshPath) {
				return "", policyError(reasonSharedHostOverlapsSSH,
					"declared shared host path %s overlaps %s", shared, sshPath)
			}
		}
	}

	if stateRoot != "" {
		// The state root normally does not exist yet, so it is judged where
		// it will really live: the canonical form of its nearest existing
		// ancestor with the missing components appended.
		state, err := prospectivePath(filepath.Clean(stateRoot))
		if err != nil {
			return "", policyError(reasonStateRootUnavailable,
				"resolve launcher private state root: %v", err)
		}
		if overlaps(shared, state) {
			return "", policyError(reasonSharedHostOverlapsState,
				"declared shared host path %s overlaps the launcher private state root %s", shared, state)
		}
	}

	return shared, nil
}

// validateChildPath judges the path the child sees. It is a path inside the
// sandbox, so it is checked lexically with slash semantics: it does not
// exist on the launcher's filesystem, and it must be exactly the path the
// declaration supplied rather than something a cleanup step would rewrite.
func validateChildPath(declared string) error {
	if declared == "" {
		return policyError(reasonChildPathInvalid, "declared shared child path is empty")
	}
	for _, r := range declared {
		if r == 0 || unicode.IsControl(r) {
			return policyError(reasonChildPathInvalid,
				"declared shared child path %q contains a control character", declared)
		}
	}
	if !strings.HasPrefix(declared, "/") {
		return policyError(reasonChildPathInvalid,
			"declared shared child path %q is not absolute", declared)
	}
	// Requiring the supplied path to already be clean rejects traversal
	// components and ambiguous duplicate separators outright, instead of
	// silently accepting a path that normalizes to something else.
	if path.Clean(declared) != declared {
		return policyError(reasonChildPathInvalid,
			"declared shared child path %q is not lexically clean", declared)
	}
	if declared == "/" {
		return policyError(reasonChildPathIsRoot, "declared shared child path is the filesystem root")
	}
	for _, reserved := range reservedChildPaths {
		if overlapsSlash(declared, reserved) {
			return policyError(reasonChildPathReserved,
				"declared shared child path %s overlaps the reserved child path %s", declared, reserved)
		}
	}
	return nil
}

// canonicalDir resolves path through symlinks and requires that it already
// exists as a directory. A path the launcher would have to create cannot be
// judged: nothing says where it will really live once another component
// creates it.
func canonicalDir(dir string) (string, error) {
	if dir == "" {
		return "", xerrors.New("path is empty")
	}
	if !filepath.IsAbs(dir) {
		return "", xerrors.Errorf("%q must be absolute", dir)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", xerrors.Errorf("resolve %s: %w", dir, err)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", xerrors.Errorf("stat %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", xerrors.Errorf("%s is not a directory", resolved)
	}
	return resolved, nil
}

// parentSSHPaths returns the parent's SSH directory as it is named under
// the canonical home, plus its target when it is a symlink. Both are
// compared, so a shared path cannot reach the owner's keys through a
// dotfiles symlink either.
func parentSSHPaths(home string) []string {
	sshPath := filepath.Join(home, ".ssh")
	sshPaths := []string{sshPath}
	if resolved, err := filepath.EvalSymlinks(sshPath); err == nil {
		if resolved = filepath.Clean(resolved); resolved != sshPath {
			sshPaths = append(sshPaths, resolved)
		}
	}
	return sshPaths
}

// overlaps reports whether two canonical paths are the same path or one
// contains the other. Both directions matter: a shared path inside the
// owner's SSH directory exposes the keys, and a shared path that contains
// it exposes them just as well.
func overlaps(a, b string) bool {
	return isWithin(a, b) || isWithin(b, a)
}

// overlapsSlash is overlaps for slash-separated child paths, which are
// judged independently of the launcher's own path syntax.
func overlapsSlash(a, b string) bool {
	return isWithinSlash(a, b) || isWithinSlash(b, a)
}

// isWithinSlash reports whether p is dir itself or lives inside it. Both
// arguments must be absolute, clean, slash-separated paths.
func isWithinSlash(p, dir string) bool {
	if p == dir {
		return true
	}
	if dir == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, dir+"/")
}
