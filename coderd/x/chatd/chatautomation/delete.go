// Package chatautomation holds the operations that the API and the agent tool
// share for chat automations, so that each rule exists once.
package chatautomation

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
)

// ErrDeleteForbidden means that the actor can read the automation and cannot
// delete it.
var ErrDeleteForbidden = xerrors.New("not allowed to delete the chat automation")

// Delete deletes an automation in one transaction. It returns the deleted row,
// with its webhook secret, for the audit entry. An automation that does not
// exist, that the actor cannot read, or that belongs to another organization
// gives an error that httpapi.Is404Error reports. An actor that can read it and
// cannot delete it gets ErrDeleteForbidden.
//
// actor needs delete on the automation.
func Delete(
	ctx context.Context,
	db database.Store,
	actor rbac.Subject,
	organizationID, id uuid.UUID,
) (database.ChatAutomation, error) {
	actorCtx := dbauthz.As(ctx, actor)

	var deleted database.ChatAutomation
	err := db.InTx(func(tx database.Store) error {
		automation, err := tx.GetChatAutomationByID(actorCtx, id)
		if err != nil {
			return err
		}
		if automation.OrganizationID != organizationID {
			return sql.ErrNoRows
		}
		err = tx.DeleteChatAutomationByID(actorCtx, id)
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrDeleteForbidden
		}
		if err != nil {
			return xerrors.Errorf("delete chat automation: %w", err)
		}
		deleted = automation
		return nil
	}, nil)
	if err != nil {
		return database.ChatAutomation{}, err
	}
	return deleted, nil
}
