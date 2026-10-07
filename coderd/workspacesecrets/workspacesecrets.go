// Package workspacesecrets decides which secrets are delivered to a
// workspace and on which targets. The agent manifest and the workspace build
// secrets API both use Resolve so they always agree.
package workspacesecrets

import (
	"path"

	"github.com/google/uuid"
)

// FilePathPolicy controls whether secrets may be delivered as files.
type FilePathPolicy int

const (
	// FilePathAllowed delivers file_path targets.
	FilePathAllowed FilePathPolicy = iota
	// FilePathBlocked drops every secret's file_path target, as set by the
	// deployment's user secret file path option. Env targets are still
	// delivered.
	FilePathBlocked
)

// Secret is a candidate for delivery to a workspace. Value may be empty when
// only metadata is needed.
type Secret struct {
	ID       uuid.UUID
	EnvName  string
	FilePath string
	// Enabled is false for disabled user secrets. It is ignored for build
	// secrets, which cannot be disabled.
	Enabled bool
	Value   string
}

// Resolved is a Secret with its delivery outcome.
type Resolved struct {
	Secret
	// DeliveredEnvName and DeliveredFilePath are the targets the secret is
	// actually delivered on. They are empty when the secret is disabled, the
	// target is unset, blocked by policy, or taken by another secret.
	DeliveredEnvName  string
	DeliveredFilePath string
	// EnvReplacedBy and FileReplacedBy hold the ID of the secret delivered
	// on this secret's target instead of it.
	EnvReplacedBy  uuid.NullUUID
	FileReplacedBy uuid.NullUUID
}

// Delivered reports whether the secret is delivered on any target.
func (r Resolved) Delivered() bool {
	return r.DeliveredEnvName != "" || r.DeliveredFilePath != ""
}

// Resolve returns the delivery outcome of the user secrets and the workspace
// build secrets, each in input order.
//
// Build secrets win over user secrets that share an env_name or file_path.
// Disabled user secrets are never delivered and never replace or get
// replaced. Env names are compared exactly. File paths are compared after
// path.Clean, so "~/a//b" and "~/a/b" collide, but "~/a" and "/home/<user>/a"
// cannot be matched on the server and do not.
func Resolve(user, build []Secret, policy FilePathPolicy) (resolvedUser, resolvedBuild []Resolved) {
	envOwner := map[string]uuid.UUID{}
	fileOwner := map[string]uuid.UUID{}
	resolve := func(s Secret, enabled bool) Resolved {
		r := Resolved{Secret: s}
		if !enabled {
			return r
		}
		if claim(envOwner, s.EnvName, s.ID, &r.EnvReplacedBy) {
			r.DeliveredEnvName = s.EnvName
		}
		if policy == FilePathAllowed && s.FilePath != "" && claim(fileOwner, path.Clean(s.FilePath), s.ID, &r.FileReplacedBy) {
			r.DeliveredFilePath = s.FilePath
		}
		return r
	}

	// Build secrets claim targets first, so they take precedence.
	resolvedBuild = make([]Resolved, len(build))
	for i, s := range build {
		resolvedBuild[i] = resolve(s, true)
	}
	resolvedUser = make([]Resolved, len(user))
	for i, s := range user {
		resolvedUser[i] = resolve(s, s.Enabled)
	}
	return resolvedUser, resolvedBuild
}

// claim assigns key to id unless another secret already holds it, in which
// case that secret's ID is stored in replacedBy.
func claim(owners map[string]uuid.UUID, key string, id uuid.UUID, replacedBy *uuid.NullUUID) bool {
	if key == "" {
		return false
	}
	if owner, taken := owners[key]; taken {
		*replacedBy = uuid.NullUUID{UUID: owner, Valid: true}
		return false
	}
	owners[key] = id
	return true
}
