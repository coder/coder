//go:build !slim

package cli

import (
	"fmt"
	"os/signal"
	"sort"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) newCreateAdminUserCommand() *clibase.Cmd {
	var (
		newUserDBURL              string
		newUserSSHKeygenAlgorithm string
		newUserUsername           string
		newUserEmail              string
		newUserPassword           string
	)
	createAdminUserCommand := &clibase.Cmd{
		Use:   "create-admin-user",
		Short: "Create a new admin user with the given username, email and password and adds it to every organization.",
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(newUserSSHKeygenAlgorithm)
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %q: %w", newUserSSHKeygenAlgorithm, err)
			}

			cfg := r.createConfig()
			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			ctx, cancel := signal.NotifyContext(ctx, InterruptSignals...)
			defer cancel()

			if newUserDBURL == "" {
				cliui.Infof(inv.Stdout, "Using built-in PostgreSQL (%s)", cfg.PostgresPath())
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
				newUserUsername, err = cliui.Prompt(inv, cliui.PromptOptions{
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
				newUserEmail, err = cliui.Prompt(inv, cliui.PromptOptions{
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
				newUserPassword, err = cliui.Prompt(inv, cliui.PromptOptions{
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
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
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

				_, _ = fmt.Fprintln(inv.Stderr, "Creating user...")
				newUser, err = tx.InsertUser(ctx, database.InsertUserParams{
					ID:             uuid.New(),
					Email:          newUserEmail,
					Username:       newUserUsername,
					HashedPassword: []byte(hashedPassword),
					CreatedAt:      dbtime.Now(),
					UpdatedAt:      dbtime.Now(),
					RBACRoles:      []string{rbac.RoleOwner()},
					LoginType:      database.LoginTypePassword,
				})
				if err != nil {
					return xerrors.Errorf("insert user: %w", err)
				}

				_, _ = fmt.Fprintln(inv.Stderr, "Generating user SSH key...")
				privateKey, publicKey, err := gitsshkey.Generate(sshKeygenAlgorithm)
				if err != nil {
					return xerrors.Errorf("generate user gitsshkey: %w", err)
				}
				_, err = tx.InsertGitSSHKey(ctx, database.InsertGitSSHKeyParams{
					UserID:     newUser.ID,
					CreatedAt:  dbtime.Now(),
					UpdatedAt:  dbtime.Now(),
					PrivateKey: privateKey,
					PublicKey:  publicKey,
				})
				if err != nil {
					return xerrors.Errorf("insert user gitsshkey: %w", err)
				}

				for _, org := range orgs {
					_, _ = fmt.Fprintf(inv.Stderr, "Adding user to organization %q (%s) as admin...\n", org.Name, org.ID.String())
					_, err := tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
						OrganizationID: org.ID,
						UserID:         newUser.ID,
						CreatedAt:      dbtime.Now(),
						UpdatedAt:      dbtime.Now(),
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

			_, _ = fmt.Fprintln(inv.Stderr, "")
			_, _ = fmt.Fprintln(inv.Stderr, "User created successfully.")
			_, _ = fmt.Fprintln(inv.Stderr, "ID:       "+newUser.ID.String())
			_, _ = fmt.Fprintln(inv.Stderr, "Username: "+newUser.Username)
			_, _ = fmt.Fprintln(inv.Stderr, "Email:    "+newUser.Email)
			_, _ = fmt.Fprintln(inv.Stderr, "Password: ********")

			return nil
		},
	}

	createAdminUserCommand.Options.Add(
		clibase.Option{
			Env:         "CODER_PG_CONNECTION_URL",
			Flag:        "postgres-url",
			Description: "URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).",
			Value:       clibase.StringOf(&newUserDBURL),
		},
		clibase.Option{
			Env:         "CODER_SSH_KEYGEN_ALGORITHM",
			Flag:        "ssh-keygen-algorithm",
			Description: "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Default:     "ed25519",
			Value:       clibase.StringOf(&newUserSSHKeygenAlgorithm),
		},
		clibase.Option{
			Env:         "CODER_USERNAME",
			Flag:        "username",
			Description: "The username of the new user. If not specified, you will be prompted via stdin.",
			Value:       clibase.StringOf(&newUserUsername),
		},
		clibase.Option{
			Env:         "CODER_EMAIL",
			Flag:        "email",
			Description: "The email of the new user. If not specified, you will be prompted via stdin.",
			Value:       clibase.StringOf(&newUserEmail),
		},
		clibase.Option{
			Env:         "CODER_PASSWORD",
			Flag:        "password",
			Description: "The password of the new user. If not specified, you will be prompted via stdin.",
			Value:       clibase.StringOf(&newUserPassword),
		},
	)

	return createAdminUserCommand
}
