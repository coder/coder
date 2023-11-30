package audit

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

type Auditor interface {
	Export(ctx context.Context, alog database.AuditLog) error
	Diff(old, new any) Map
}

type AdditionalFields struct {
	WorkspaceName  string               `json:"workspace_name"`
	BuildNumber    string               `json:"build_number"`
	BuildReason    database.BuildReason `json:"build_reason"`
	WorkspaceOwner string               `json:"workspace_owner"`
}

func NewNop() Auditor {
	return nop{}
}

type nop struct{}

func (nop) Export(context.Context, database.AuditLog) error {
	return nil
}

func (nop) Diff(any, any) Map {
	return Map{}
}
