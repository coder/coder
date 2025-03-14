//go:build !slim
package cli
import (
	"errors"
	"fmt"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/database/awsiamrds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/userpassword"
)
func (*RootCmd) resetPassword() *serpent.Command {
	var (
		postgresURL  string
		postgresAuth string
	)
	root := &serpent.Command{
		Use:        "reset-password <username>",
		Short:      "Directly connect to the database to reset a user's password",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			username := inv.Args[0]
			logger := slog.Make(sloghuman.Sink(inv.Stdout))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}
			sqlDriver := "postgres"
			if codersdk.PostgresAuth(postgresAuth) == codersdk.PostgresAuthAWSIAMRDS {
				var err error
				sqlDriver, err = awsiamrds.Register(inv.Context(), sqlDriver)
				if err != nil {
					return fmt.Errorf("register aws rds iam auth: %w", err)
				}
			}
			sqlDB, err := ConnectToPostgres(inv.Context(), logger, sqlDriver, postgresURL, nil)
			if err != nil {
				return fmt.Errorf("dial postgres: %w", err)
			}
			defer sqlDB.Close()
			db := database.New(sqlDB)
			user, err := db.GetUserByEmailOrUsername(inv.Context(), database.GetUserByEmailOrUsernameParams{
				Username: username,
			})
			if err != nil {
				return fmt.Errorf("retrieving user: %w", err)
			}
			password, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:   "Enter new " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
				Secret: true,
				Validate: func(s string) error {
					return userpassword.Validate(s)
				},
			})
			if err != nil {
				return fmt.Errorf("password prompt: %w", err)
			}
			confirmedPassword, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:     "Confirm " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
				Secret:   true,
				Validate: cliui.ValidateNotEmpty,
			})
			if err != nil {
				return fmt.Errorf("confirm password prompt: %w", err)
			}
			if password != confirmedPassword {
				return errors.New("Passwords do not match")
			}
			hashedPassword, err := userpassword.Hash(password)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			err = db.UpdateUserHashedPassword(inv.Context(), database.UpdateUserHashedPasswordParams{
				ID:             user.ID,
				HashedPassword: []byte(hashedPassword),
			})
			if err != nil {
				return fmt.Errorf("updating password: %w", err)
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
		serpent.Option{
			Name:        "Postgres Connection Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-connection-auth",
			Env:         "CODER_PG_CONNECTION_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&postgresAuth, codersdk.PostgresAuthDrivers...),
		},
	}
	return root
}
