package createusers

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	user codersdk.User
}

type User struct {
	codersdk.User
	SessionToken string
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) RunReturningUser(ctx context.Context, id string, logs io.Writer) (User, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	if r.cfg.Username == "" || r.cfg.Email == "" {
		genUsername, genEmail, err := loadtestutil.GenerateUserIdentifier(id)
		if err != nil {
			return User{}, xerrors.Errorf("generate user identifier: %w", err)
		}
		if r.cfg.Username == "" {
			r.cfg.Username = genUsername
		}
		if r.cfg.Email == "" {
			r.cfg.Email = genEmail
		}
	}

	_, _ = fmt.Fprintln(logs, "Generating user password...")
	password, err := cryptorand.String(16)
	if err != nil {
		return User{}, xerrors.Errorf("generate random password for user: %w", err)
	}

	_, _ = fmt.Fprintln(logs, "Creating user:")
	user, err := r.client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		OrganizationIDs: []uuid.UUID{r.cfg.OrganizationID},
		Username:        r.cfg.Username,
		Email:           r.cfg.Email,
		Password:        password,
	})
	if err != nil {
		return User{}, xerrors.Errorf("create user: %w", err)
	}
	r.user = user

	_, _ = fmt.Fprintln(logs, "\nLogging in as new user...")
	client := codersdk.New(r.client.URL)
	loginRes, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    r.cfg.Email,
		Password: password,
	})
	if err != nil {
		return User{}, xerrors.Errorf("login as new user: %w", err)
	}

	_, _ = fmt.Fprintf(logs, "\tOrg ID:   %s\n", r.cfg.OrganizationID.String())
	_, _ = fmt.Fprintf(logs, "\tUsername: %s\n", user.Username)
	_, _ = fmt.Fprintf(logs, "\tEmail:    %s\n", user.Email)
	_, _ = fmt.Fprintf(logs, "\tPassword: ****************\n")

	return User{User: user, SessionToken: loginRes.SessionToken}, nil
}

func (r *Runner) Cleanup(ctx context.Context, _ string, logs io.Writer) error {
	if r.user.ID != uuid.Nil {
		err := r.client.DeleteUser(ctx, r.user.ID)
		if err != nil {
			_, _ = fmt.Fprintf(logs, "failed to delete user %q: %v\n", r.user.ID.String(), err)
			return xerrors.Errorf("delete user: %w", err)
		}
	}
	return nil
}
