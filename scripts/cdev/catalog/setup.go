package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultAdminEmail    = "admin@coder.com"
	defaultAdminUsername = "admin"
	defaultAdminName     = "Admin User"
	defaultAdminPassword = "SomeSecurePassword!"

	defaultMemberEmail    = "member@coder.com"
	defaultMemberUsername = "member"
	defaultMemberName     = "Regular User"
)

// SetupResult contains the credentials for the created users.
type SetupResult struct {
	// AdminEmail is the email of the admin user.
	AdminEmail string
	// AdminUsername is the username of the admin user.
	AdminUsername string
	// AdminPassword is the password for both admin and member users.
	AdminPassword string
	// MemberEmail is the email of the regular member user.
	MemberEmail string
	// MemberUsername is the username of the regular member user.
	MemberUsername string
	// SessionToken is the admin session token for API access.
	SessionToken string
}

var _ Service[SetupResult] = (*Setup)(nil)

func OnSetup() string {
	return (&Setup{}).Name()
}

// Setup creates the first user and a regular member user for the Coder
// deployment. This is a one-shot service that runs after coderd is ready.
type Setup struct {
	result SetupResult
}

func NewSetup() *Setup {
	return &Setup{}
}

func (*Setup) Name() string {
	return "setup"
}

func (*Setup) Emoji() string {
	return "👤"
}

func (*Setup) DependsOn() []string {
	return []string{
		OnCoderd(),
	}
}

func (s *Setup) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	coderd, ok := c.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
	}
	coderdResult := coderd.Result()

	coderdURL, err := url.Parse(coderdResult.URL)
	if err != nil {
		return xerrors.Errorf("parse coderd URL: %w", err)
	}
	client := codersdk.New(coderdURL)

	pg, ok := c.MustGet(OnPostgres()).(*Postgres)
	if !ok {
		return xerrors.New("unexpected type for Postgres service")
	}

	err = pg.waitForMigrations(ctx, logger)
	if err != nil {
		return xerrors.Errorf("wait for postgres migrations: %w", err)
	}

	// Check if first user already exists by trying to get build info.
	// If users exist, we can still try to login.
	hasFirstUser, err := client.HasFirstUser(ctx)
	if err != nil {
		return xerrors.Errorf("check first user: %w", err)
	}

	s.result = SetupResult{
		AdminEmail:     defaultAdminEmail,
		AdminUsername:  defaultAdminUsername,
		AdminPassword:  defaultAdminPassword,
		MemberEmail:    defaultMemberEmail,
		MemberUsername: defaultMemberUsername,
	}

	if !hasFirstUser {
		// Create the first admin user.
		logger.Info(ctx, "creating first admin user",
			slog.F("email", defaultAdminEmail),
			slog.F("username", defaultAdminUsername))

		_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:    defaultAdminEmail,
			Username: defaultAdminUsername,
			Name:     defaultAdminName,
			Password: defaultAdminPassword,
			Trial:    false,
		})
		if err != nil {
			return xerrors.Errorf("create first user: %w", err)
		}
		logger.Info(ctx, "first admin user created successfully")
	} else {
		logger.Info(ctx, "first user already exists, skipping creation")
	}

	// Login to get a session token.
	logger.Info(ctx, "logging in as admin user")
	loginResp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    defaultAdminEmail,
		Password: defaultAdminPassword,
	})
	if err != nil {
		return xerrors.Errorf("login as admin: %w", err)
	}
	client.SetSessionToken(loginResp.SessionToken)
	s.result.SessionToken = loginResp.SessionToken

	// Check if member user already exists.
	memberExists := false
	_, err = client.User(ctx, defaultMemberUsername)
	if err == nil {
		memberExists = true
	} else {
		var sdkErr *codersdk.Error
		if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
			memberExists = false
		} else if sdkErr.StatusCode() == http.StatusBadRequest {
			// https://github.com/coder/coder/pull/22069 fixes this bug
			memberExists = false
		} else {
			return xerrors.Errorf("check member user: %w", err)
		}

	}

	if !memberExists {
		org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
		if err != nil {
			return xerrors.Errorf("get default organization: %w", err)
		}

		// Create a regular member user.
		logger.Info(ctx, "creating regular member user",
			slog.F("email", defaultMemberEmail),
			slog.F("username", defaultMemberUsername))

		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           defaultMemberEmail,
			Username:        defaultMemberUsername,
			Name:            defaultMemberName,
			Password:        defaultAdminPassword,
			UserLoginType:   codersdk.LoginTypePassword,
			UserStatus:      nil,
			OrganizationIDs: []uuid.UUID{org.ID},
		})
		if err != nil {
			return xerrors.Errorf("create member user: %w", err)
		}
		logger.Info(ctx, "regular member user created successfully")
	} else {
		logger.Info(ctx, "member user already exists, skipping creation")
	}

	logger.Info(ctx, "setup completed",
		slog.F("admin_email", s.result.AdminEmail),
		slog.F("admin_username", s.result.AdminUsername),
		slog.F("member_email", s.result.MemberEmail),
		slog.F("member_username", s.result.MemberUsername))

	return nil
}

func (*Setup) Stop(_ context.Context) error {
	// Setup is a one-shot task, nothing to stop.
	return nil
}

func (s *Setup) Result() SetupResult {
	return s.result
}
