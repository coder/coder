//go:build !slim

package cli

import (
	"database/sql"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/userpassword"
)

func (*RootCmd) resetPassword() *serpent.Cmd {
	var postgresURL string

	root := &serpent.Cmd{
		Use:        "reset-password <username>",
		Short:      "Directly connect to the database to reset a user's password",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			username := inv.Args[0]

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

			password, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:   "Enter new " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
				Secret: true,
				Validate: func(s string) error {
					return userpassword.Validate(s)
				},
			})
			if err != nil {
				return xerrors.Errorf("password prompt: %w", err)
			}
			confirmedPassword, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:     "Confirm " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
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

			_, _ = fmt.Fprintf(inv.Stdout, "\nPassword has been reset for user %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, user.Username))
			return nil
		},
	}

	root.Options = serpent.OptionSet{
		{
			Flag:        "postgres-url",
			Description: "URL of a PostgreSQL database to connect to.",
			Env:         "CODER_PG_CONNECTION_URL",
			Value:       serpent.StringOf(&postgresURL),
		},
	}

	return root
}
