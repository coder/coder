package createusers

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	userID       uuid.UUID
	sessionToken string
	user         codersdk.User
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	if r.cfg.Username == "" || r.cfg.Email == "" {
		genUsername, genEmail, err := loadtestutil.GenerateUserIdentifier(id)
		if err != nil {
			return xerrors.Errorf("generate user identifier: %w", err)
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
		return xerrors.Errorf("generate random password for user: %w", err)
	}

	_, _ = fmt.Fprintln(logs, "Creating user:")
	user, err := r.client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		OrganizationIDs: []uuid.UUID{r.cfg.OrganizationID},
		Username:        r.cfg.Username,
		Email:           r.cfg.Email,
		Password:        password,
	})
	if err != nil {
		return xerrors.Errorf("create user: %w", err)
	}
	r.userID = user.ID
	r.user = user

	_, _ = fmt.Fprintln(logs, "\nLogging in as new user...")
	client := codersdk.New(r.client.URL)
	loginRes, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    r.cfg.Email,
		Password: password,
	})
	if err != nil {
		return xerrors.Errorf("login as new user: %w", err)
	}
	r.sessionToken = loginRes.SessionToken

	_, _ = fmt.Fprintf(logs, "\tOrg ID:   %s\n", r.cfg.OrganizationID.String())
	_, _ = fmt.Fprintf(logs, "\tUsername: %s\n", user.Username)
	_, _ = fmt.Fprintf(logs, "\tEmail:    %s\n", user.Email)
	_, _ = fmt.Fprintf(logs, "\tPassword: ****************\n")

	return nil
}

func (r *Runner) Cleanup(ctx context.Context, _ string, logs io.Writer) error {
	if r.userID != uuid.Nil {
		err := r.client.DeleteUser(ctx, r.userID)
		if err != nil {
			_, _ = fmt.Fprintf(logs, "failed to delete user %q: %v\n", r.userID.String(), err)
			return xerrors.Errorf("delete user: %w", err)
		}
	}
	return nil
}

func (r *Runner) SessionToken() string {
	return r.sessionToken
}

func (r *Runner) User() codersdk.User {
	return r.user
}
