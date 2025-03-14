package dbauthz
import (
	"fmt"
	"errors"
	"context"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	agpldbz "github.com/coder/coder/v2/coderd/database/dbauthz"
)
type EnterpriseTemplateAccessControlStore struct{}
func (EnterpriseTemplateAccessControlStore) GetTemplateAccessControl(t database.Template) agpldbz.TemplateAccessControl {
	return agpldbz.TemplateAccessControl{
		RequireActiveVersion: t.RequireActiveVersion,
		Deprecated:           t.Deprecated,
	}
}
func (EnterpriseTemplateAccessControlStore) SetTemplateAccessControl(ctx context.Context, store database.Store, id uuid.UUID, opts agpldbz.TemplateAccessControl) error {
	err := store.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
		ID:                   id,
		RequireActiveVersion: opts.RequireActiveVersion,
		Deprecated:           opts.Deprecated,
	})
	if err != nil {
		return fmt.Errorf("update template access control: %w", err)
	}
	return nil
}
