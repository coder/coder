// This program generates an input.json file containing action, object, and subject fields
// to be used as input for `opa eval`, e.g.:
// > opa eval --format=pretty "data.authz.allow" -d policy.rego -i input.json
// This helps verify that the policy returns the expected authorization decision.
package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

type SubjectJSON struct {
	ID     string      `json:"id"`
	Roles  []rbac.Role `json:"roles"`
	Groups []string    `json:"groups"`
	Scope  rbac.Scope  `json:"scope"`
}
type OutputData struct {
	Action  policy.Action `json:"action"`
	Object  rbac.Object   `json:"object"`
	Subject *SubjectJSON  `json:"subject"`
}

func newSubjectJSON(s rbac.Subject) (*SubjectJSON, error) {
	roles, err := s.Roles.Expand()
	if err != nil {
		return nil, xerrors.Errorf("failed to expand subject roles: %w", err)
	}
	scopes, err := s.Scope.Expand()
	if err != nil {
		return nil, xerrors.Errorf("failed to expand subject scopes: %w", err)
	}
	return &SubjectJSON{
		ID:     s.ID,
		Roles:  roles,
		Groups: s.Groups,
		Scope:  scopes,
	}, nil
}

// TODO: Support optional CLI flags to customize the input:
// --action=[one of the supported actions]
// --subject=[one of the built-in roles]
// --object=[one of the supported resources]
func main() {
	// Template Admin user
	subject := rbac.Subject{
		FriendlyName: "Test Name",
		Email:        "test@coder.com",
		Type:         "user",
		ID:           uuid.New().String(),
		Roles: rbac.RoleIdentifiers{
			rbac.RoleTemplateAdmin(),
		},
		Scope: rbac.ScopeAll,
	}

	subjectJSON, err := newSubjectJSON(subject)
	if err != nil {
		log.Fatalf("Failed to convert to subject to JSON: %v", err)
	}

	// Delete action
	action := policy.ActionDelete

	// Prebuilt Workspace object
	object := rbac.Object{
		ID:    uuid.New().String(),
		Owner: "c42fdf75-3097-471c-8c33-fb52454d81c0",
		OrgID: "663f8241-23e0-41c4-a621-cec3a347318e",
		Type:  "prebuilt_workspace",
	}

	// Output file path
	outputPath := "input.json"

	output := OutputData{
		Action:  action,
		Object:  object,
		Subject: subjectJSON,
	}

	outputBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal output to json: %v", err)
	}

	if err := os.WriteFile(outputPath, outputBytes, 0o600); err != nil {
		log.Fatalf("Failed to generate input file: %v", err)
	}

	log.Println("Input JSON written to", outputPath)
}
