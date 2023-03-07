package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/migrations"
	"github.com/coder/coder/coderd/userpassword"
)

func resetPassword() *clibase.Command {
	var postgresURL string

	root := &clibase.Command{
		Use:   "reset-password <username>",
		Short: "Directly connect to the database to reset a user's password",
		Args:  cobra.ExactArgs(1),
		Handler: func(inv *clibase.Invokation) error {
			username := args[0]

			sqlDB, err := sql.Open("postgres", postgresURL)
			if err != nil {
				return xerrors.Errorf("dial postgres: %w", err)
			}
			defer sqlDB.Close()
			err = sqlDB.Ping()
			if err != nil {
				return xerrors.Errorf("ping postgres: %w", err)
			}

			err = migrations.EnsureClean(sqlDB)
			if err != nil {
				return xerrors.Errorf("database needs migration: %w", err)
			}
			db := database.New(sqlDB)

			user, err := db.GetUserByEmailOrUsername(inv.Context(), database.GetUserByEmailOrUsernameParams{
				Username: username,
			})
			if err != nil {
				return xerrors.Errorf("retrieving user: %w", err)
			}

			password, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:   "Enter new " + cliui.Styles.Field.Render("password") + ":",
				Secret: true,
				Validate: func(s string) error {
					return userpassword.Validate(s)
				},
			})
			if err != nil {
				return xerrors.Errorf("password prompt: %w", err)
			}
			confirmedPassword, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:     "Confirm " + cliui.Styles.Field.Render("password") + ":",
				Secret:   true,
				Validate: cliui.ValidateNotEmpty,
			})
			if err != nil {
				return xerrors.Errorf("confirm password prompt: %w", err)
			}
			if password != confirmedPassword {
				return xerrors.New("Passwords do not match")
			}

			hashedPassword, err := userpassword.Hash(password)
			if err != nil {
				return xerrors.Errorf("hash password: %w", err)
			}

			err = db.UpdateUserHashedPassword(inv.Context(), database.UpdateUserHashedPasswordParams{
				ID:             user.ID,
				HashedPassword: []byte(hashedPassword),
			})
			if err != nil {
				return xerrors.Errorf("updating password: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPassword has been reset for user %s!\n", cliui.Styles.Keyword.Render(user.Username))
			return nil
		},
	}

	cliflag.StringVarP(root.Flags(), &postgresURL, "postgres-url", "", "CODER_PG_CONNECTION_URL", "", "URL of a PostgreSQL database to connect to")

	return root
}
