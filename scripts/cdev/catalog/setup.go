package catalog

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

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

func OnSetup() ServiceName {
	return (&Setup{}).Name()
}

// Setup creates the first user and a regular member user for the Coder
// deployment. This is a one-shot service that runs after coderd is ready.
type Setup struct {
	currentStep atomic.Pointer[string]
	result      SetupResult
}

func (s *Setup) CurrentStep() string {
	if st := s.currentStep.Load(); st != nil {
		return *st
	}
	return ""
}

func (s *Setup) setStep(step string) {
	s.currentStep.Store(&step)
}

func NewSetup() *Setup {
	return &Setup{}
}

func (*Setup) Name() ServiceName {
	return CDevSetup
}

func (*Setup) Emoji() string {
	return "👤"
}

func (*Setup) DependsOn() []ServiceName {
	return []ServiceName{
		OnCoderd(),
	}
}

func (s *Setup) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	defer s.setStep("")

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
		s.setStep("Creating first admin user")
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
	s.setStep("Logging in as admin")
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
		s.setStep("Creating member user")
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

	// Create docker template if it doesn't exist.
	s.setStep("Creating docker template")
	if err := s.createDockerTemplate(ctx, logger, client); err != nil {
		// Don't fail setup if template creation fails - it's not critical.
		logger.Warn(ctx, "failed to create docker template", slog.Error(err))
	}

	logger.Info(ctx, "setup completed",
		slog.F("admin_email", s.result.AdminEmail),
		slog.F("admin_username", s.result.AdminUsername),
		slog.F("member_email", s.result.MemberEmail),
		slog.F("member_username", s.result.MemberUsername))

	return nil
}

func (s *Setup) createDockerTemplate(ctx context.Context, logger slog.Logger, client *codersdk.Client) error {
	const templateName = "docker"

	// Check if template already exists.
	org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return xerrors.Errorf("get default organization: %w", err)
	}

	_, err = client.TemplateByName(ctx, org.ID, templateName)
	if err == nil {
		logger.Info(ctx, "docker template already exists, skipping creation")
		return nil
	}

	// Template doesn't exist, create it.
	logger.Info(ctx, "creating docker template")

	// Copy template to temp directory and run terraform init to generate lock file.
	s.setStep("Initializing terraform providers")
	templateDir := filepath.Join("examples", "templates", "docker")
	tempDir, err := s.prepareTemplateDir(ctx, logger, templateDir)
	if err != nil {
		return xerrors.Errorf("prepare template directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a tar archive of the initialized template files.
	tarData, err := createTarFromDir(tempDir)
	if err != nil {
		return xerrors.Errorf("create tar archive: %w", err)
	}

	// Upload the template files.
	s.setStep("Uploading template files")
	uploadResp, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(tarData))
	if err != nil {
		return xerrors.Errorf("upload template files: %w", err)
	}

	// Create a template version.
	s.setStep("Creating template version")
	version, err := client.CreateTemplateVersion(ctx, org.ID, codersdk.CreateTemplateVersionRequest{
		Name:          "v1.0.0",
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		FileID:        uploadResp.ID,
		Provisioner:   codersdk.ProvisionerTypeTerraform,
	})
	if err != nil {
		return xerrors.Errorf("create template version: %w", err)
	}

	// Wait for the template version to be ready.
	s.setStep("Waiting for template to build")
	version, err = s.waitForTemplateVersion(ctx, logger, client, version.ID)
	if err != nil {
		return xerrors.Errorf("wait for template version: %w", err)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		logger.Error(ctx, "template version build failed", slog.F("error", version.Job.Error))
		return xerrors.Errorf("template version failed: %s", version.Job.Status)
	}

	// Create the template.
	s.setStep("Finalizing template")
	_, err = client.CreateTemplate(ctx, org.ID, codersdk.CreateTemplateRequest{
		Name:        templateName,
		DisplayName: "Docker",
		Description: "Develop in Docker containers",
		Icon:        "/icon/docker.png",
		VersionID:   version.ID,
	})
	if err != nil {
		return xerrors.Errorf("create template: %w", err)
	}

	logger.Info(ctx, "docker template created successfully")
	return nil
}

// prepareTemplateDir copies the template to a temp directory and runs terraform init
// to generate the lock file that Coder's provisioner needs.
func (s *Setup) prepareTemplateDir(ctx context.Context, logger slog.Logger, srcDir string) (string, error) {
	// Create temp directory.
	tempDir, err := os.MkdirTemp("", "cdev-template-*")
	if err != nil {
		return "", xerrors.Errorf("create temp dir: %w", err)
	}

	// Copy all files from source to temp directory.
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(tempDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file.
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})
	if err != nil {
		os.RemoveAll(tempDir)
		return "", xerrors.Errorf("copy template files: %w", err)
	}

	// Run terraform init to download providers and create lock file.
	logger.Info(ctx, "running terraform init", slog.F("dir", tempDir))

	cmd := exec.CommandContext(ctx, "terraform", "init")
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tempDir)
		return "", xerrors.Errorf("terraform init failed: %w\nOutput: %s", err, string(output))
	}

	logger.Debug(ctx, "terraform init completed", slog.F("output", string(output)))

	// Remove the .terraform directory - we only need the lock file.
	// The provisioner will download the providers itself.
	tfDir := filepath.Join(tempDir, ".terraform")
	if err := os.RemoveAll(tfDir); err != nil {
		logger.Warn(ctx, "failed to remove .terraform directory", slog.Error(err))
	}

	return tempDir, nil
}

func (s *Setup) waitForTemplateVersion(ctx context.Context, logger slog.Logger, client *codersdk.Client, versionID uuid.UUID) (codersdk.TemplateVersion, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return codersdk.TemplateVersion{}, ctx.Err()
		case <-timeout:
			return codersdk.TemplateVersion{}, xerrors.New("timeout waiting for template version")
		case <-ticker.C:
			version, err := client.TemplateVersion(ctx, versionID)
			if err != nil {
				logger.Warn(ctx, "failed to get template version", slog.Error(err))
				continue
			}

			if !version.Job.Status.Active() {
				return version, nil
			}

			logger.Debug(ctx, "template version still building",
				slog.F("status", version.Job.Status))
		}
	}
}

// createTarFromDir creates a tar archive from a directory.
func createTarFromDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		// Get relative path.
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Create tar header.
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file content.
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (*Setup) Stop(_ context.Context) error {
	// Setup is a one-shot task, nothing to stop.
	return nil
}

func (s *Setup) Result() SetupResult {
	return s.result
}
