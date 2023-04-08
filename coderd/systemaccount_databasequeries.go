package coderd

import (
	"database/sql"

	"github.com/google/uuid"
)

/***
Added this temporary file for database interactions as "Create_migrations" did not work during assignment

Improvements: Start transcation for each operations where more than one table are modified, to be reverted in case of failure

**/

func deleteSystemAccountQuery(id UUID) error {
	_, err = db.Exec(`
	DELETE FROM system_accounts WHERE id = $1
`, id)

	return err

}

func updateSystemAccount(id UUID, updateAt time, name string) error {
	_, err = db.Exec(`
            UPDATE system_accounts SET name = $1, updated_at = $2 WHERE id = $3
        `, name, updateAt, id)

	return err
}

func createSystemAccountQuery(account *SystemAccount) {
	_, err := db.Exec(`
        INSERT INTO system_accounts (id, name, created_at, updated_at, organization_id, created_by)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, account.ID, account.Name, account.CreatedAt, account.UpdatedAt, account.OrganizationID, account.CreatedBy)

	return err
}

func getSystemAccount(id uuid.UUID) (*SystemAccount, error) {
	account := &SystemAccount{}
	if err := db.QueryRow("SELECT * FROM system_accounts WHERE id = $1", id).Scan(
		&account.ID,
		&account.Name,
		&account.CreatedAt,
		&account.UpdatedAt,
		&account.OrganizationID,
		&account.CreatedBy,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}
	return account, nil
}
