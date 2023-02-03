//go:build !slim

package cli

import (
	"fmt"
	"os"
	"os/signal"
	"sort"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/codersdk"
)

func newCreateAdminUserCommand() *cobra.Command {
	var (
		newUserDBURL              string
		newUserSSHKeygenAlgorithm string
		newUserUsername           string
		newUserEmail              string
		newUserPassword           string
	)
	createAdminUserCommand := &cobra.Command{
		Use:   "create-admin-user",
		Short: "Create a new admin user with the given username, email and password and adds it to every organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(newUserSSHKeygenAlgorithm)
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %q: %w", newUserSSHKeygenAlgorithm, err)
			}

			if val, exists := os.LookupEnv("CODER_POSTGRES_URL"); exists {
				newUserDBURL = val
			}
			if val, exists := os.LookupEnv("CODER_SSH_KEYGEN_ALGORITHM"); exists {
				newUserSSHKeygenAlgorithm = val
			}
			if val, exists := os.LookupEnv("CODER_USERNAME"); exists {
				newUserUsername = val
			}
			if val, exists := os.LookupEnv("CODER_EMAIL"); exists {
				newUserEmail = val
			}
			if val, exists := os.LookupEnv("CODER_PASSWORD"); exists {
				newUserPassword = val
			}

			cfg := createConfig(cmd)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			ctx, cancel := signal.NotifyContext(ctx, InterruptSignals...)
			defer cancel()

			if newUserDBURL == "" {
				cmd.Printf("Using built-in PostgreSQL (%s)\n", cfg.PostgresPath())
				url, closePg, err := startBuiltinPostgres(ctx, cfg, logger)
				if err != nil {
					return err
				}
				defer func() {
					_ = closePg()
				}()
				newUserDBURL = url
			}

			sqlDB, err := connectToPostgres(ctx, logger, "postgres", newUserDBURL)
			if err != nil {
				return xerrors.Errorf("connect to postgres: %w", err)
			}
			defer func() {
				_ = sqlDB.Close()
			}()
			db := database.New(sqlDB)

			validateInputs := func(username, email, password string) error {
				// Use the validator tags so we match the API's validation.
				req := codersdk.CreateUserRequest{
					Username:       "username",
					Email:          "email@coder.com",
					Password:       "ValidPa$$word123!",
					OrganizationID: uuid.New(),
				}
				if username != "" {
					req.Username = username
				}
				if email != "" {
					req.Email = email
				}
				if password != "" {
					req.Password = password
				}

				return httpapi.Validate.Struct(req)
			}

			if newUserUsername == "" {
				newUserUsername, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text: "Username",
					Validate: func(val string) error {
						if val == "" {
							return xerrors.New("username cannot be empty")
						}
						return validateInputs(val, "", "")
					},
				})
				if err != nil {
					return err
				}
			}
			if newUserEmail == "" {
				newUserEmail, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text: "Email",
					Validate: func(val string) error {
						if val == "" {
							return xerrors.New("email cannot be empty")
						}
						return validateInputs("", val, "")
					},
				})
				if err != nil {
					return err
				}
			}
			if newUserPassword == "" {
				newUserPassword, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:   "Password",
					Secret: true,
					Validate: func(val string) error {
						if val == "" {
							return xerrors.New("password cannot be empty")
						}
						return validateInputs("", "", val)
					},
				})
				if err != nil {
					return err
				}

				// Prompt again.
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:   "Confirm password",
					Secret: true,
					Validate: func(val string) error {
						if val != newUserPassword {
							return xerrors.New("passwords do not match")
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}

			err = validateInputs(newUserUsername, newUserEmail, newUserPassword)
			if err != nil {
				return xerrors.Errorf("validate inputs: %w", err)
			}

			hashedPassword, err := userpassword.Hash(newUserPassword)
			if err != nil {
				return xerrors.Errorf("hash password: %w", err)
			}

			// Create the user.
			var newUser database.User
			err = db.InTx(func(tx database.Store) error {
				orgs, err := tx.GetOrganizations(ctx)
				if err != nil {
					return xerrors.Errorf("get organizations: %w", err)
				}

				// Sort organizations by name so that test output is consistent.
				sort.Slice(orgs, func(i, j int) bool {
					return orgs[i].Name < orgs[j].Name
				})

				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Creating user...")
				newUser, err = tx.InsertUser(ctx, database.InsertUserParams{
					ID:             uuid.New(),
					Email:          newUserEmail,
					Username:       newUserUsername,
					HashedPassword: []byte(hashedPassword),
					CreatedAt:      database.Now(),
					UpdatedAt:      database.Now(),
					RBACRoles:      []string{rbac.RoleOwner()},
					LoginType:      database.LoginTypePassword,
				})
				if err != nil {
					return xerrors.Errorf("insert user: %w", err)
				}

				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Generating user SSH key...")
				privateKey, publicKey, err := gitsshkey.Generate(sshKeygenAlgorithm)
				if err != nil {
					return xerrors.Errorf("generate user gitsshkey: %w", err)
				}
				_, err = tx.InsertGitSSHKey(ctx, database.InsertGitSSHKeyParams{
					UserID:     newUser.ID,
					CreatedAt:  database.Now(),
					UpdatedAt:  database.Now(),
					PrivateKey: privateKey,
					PublicKey:  publicKey,
				})
				if err != nil {
					return xerrors.Errorf("insert user gitsshkey: %w", err)
				}

				for _, org := range orgs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Adding user to organization %q (%s) as admin...\n", org.Name, org.ID.String())
					_, err := tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
						OrganizationID: org.ID,
						UserID:         newUser.ID,
						CreatedAt:      database.Now(),
						UpdatedAt:      database.Now(),
						Roles:          []string{rbac.RoleOrgAdmin(org.ID)},
					})
					if err != nil {
						return xerrors.Errorf("insert organization member: %w", err)
					}
				}

				return nil
			}, nil)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "User created successfully.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ID:       "+newUser.ID.String())
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Username: "+newUser.Username)
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Email:    "+newUser.Email)
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Password: ********")

			return nil
		},
	}
	createAdminUserCommand.Flags().StringVar(&newUserDBURL, "postgres-url", "", "URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case). Consumes $CODER_POSTGRES_URL.")
	createAdminUserCommand.Flags().StringVar(&newUserSSHKeygenAlgorithm, "ssh-keygen-algorithm", "ed25519", "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\". Consumes $CODER_SSH_KEYGEN_ALGORITHM.")
	createAdminUserCommand.Flags().StringVar(&newUserUsername, "username", "", "The username of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_USERNAME.")
	createAdminUserCommand.Flags().StringVar(&newUserEmail, "email", "", "The email of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_EMAIL.")
	createAdminUserCommand.Flags().StringVar(&newUserPassword, "password", "", "The password of the new user. If not specified, you will be prompted via stdin. Consumes $CODER_PASSWORD.")

	return createAdminUserCommand
}
