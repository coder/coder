// Package workspacesecrets decides which secrets are delivered to a
// workspace and on which targets. The agent manifest and the secrets API
// both use Resolve so they always agree.
package workspacesecrets

import (
	"github.com/google/uuid"
)

// FilePathPolicy controls whether secrets may be delivered as files.
type FilePathPolicy int

const (
	FilePathAllowed FilePathPolicy = iota
	FilePathBlocked
)

// Source identifies where a secret delivered to a workspace comes from.
type Source int

const (
	// SourceUser is a user secret, shared by all of the user's workspaces.
	SourceUser Source = iota
	// SourceBuild is a workspace secret set on a workspace build.
	SourceBuild
)

// Secret is a candidate for delivery to a workspace. Value may be empty when
// only metadata is needed.
type Secret struct {
	ID       uuid.UUID
	Source   Source
	Name     string
	EnvName  string
	FilePath string
	// Enabled is false for disabled user secrets. Workspace secrets are
	// always enabled.
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
	// EnvReplacedBy and FileReplacedBy name the secret delivered on this
	// secret's target instead of it.
	EnvReplacedBy  uuid.NullUUID
	FileReplacedBy uuid.NullUUID
}

// Delivered reports whether the secret is delivered on any target.
func (r Resolved) Delivered() bool {
	return r.DeliveredEnvName != "" || r.DeliveredFilePath != ""
}

// Resolve returns the delivery outcome of each secret, in input order.
//
// Disabled secrets are never delivered and never replace or get replaced.
// When secrets share an env_name or file_path, a workspace build secret wins
// over a user secret. Targets are compared exactly as written.
func Resolve(secrets []Secret, policy FilePathPolicy) []Resolved {
	out := make([]Resolved, len(secrets))
	envOwner := map[string]uuid.UUID{}
	fileOwner := map[string]uuid.UUID{}

	// Claim targets in precedence order: build secrets first.
	for _, pass := range []Source{SourceBuild, SourceUser} {
		for i, s := range secrets {
			if s.Source != pass {
				continue
			}
			r := Resolved{Secret: s}
			if s.Enabled {
				r.DeliveredEnvName, r.EnvReplacedBy = claim(envOwner, s.EnvName, s.ID)
				if policy == FilePathAllowed {
					r.DeliveredFilePath, r.FileReplacedBy = claim(fileOwner, s.FilePath, s.ID)
				}
			}
			out[i] = r
		}
	}
	return out
}

// claim assigns target to id unless another secret already holds it.
func claim(owners map[string]uuid.UUID, target string, id uuid.UUID) (string, uuid.NullUUID) {
	if target == "" {
		return "", uuid.NullUUID{}
	}
	if owner, taken := owners[target]; taken {
		return "", uuid.NullUUID{UUID: owner, Valid: true}
	}
	owners[target] = id
	return target, uuid.NullUUID{}
}
