package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// Default credentials for development environment.
const (
	defaultAdminEmail    = "admin@coder.com"
	defaultAdminUsername = "admin"
	defaultAdminName     = "Admin User"
	defaultAdminPassword = "SomeSecurePassword!"

	defaultMemberEmail    = "member@coder.com"
	defaultMemberUsername = "member"
	defaultMemberName     = "Regular User"
)

func main() {
	coderURL := os.Getenv("CODER_URL")
	if coderURL == "" {
		coderURL = "http://coderd:3000"
	}

	log.Printf("Running cdevsetup against %s", coderURL)

	ctx := context.Background()
	if err := run(ctx, coderURL); err != nil {
		log.Fatalf("Setup failed: %v", err)
	}

	log.Println("cdevsetup complete!")
	if os.Args[1] == "remain" {
		for {
			time.Sleep(10 * time.Second)
			// For docker compose shit
			log.Println("cdevsetup was told to infinitely remain running, so here we are...")
		}
	}
}

func run(ctx context.Context, coderURLStr string) error {
	coderURL, err := url.Parse(coderURLStr)
	if err != nil {
		return xerrors.Errorf("parse coderd URL: %w", err)
	}
	client := codersdk.New(coderURL)

	for {
		_, err := client.HasFirstUser(ctx)
		if err != nil {
			log.Printf("failed to check if user exists, will retry: %v", err)
			time.Sleep(time.Second)
		}
		break
	}

	// Check if first user already exists.
	hasFirstUser, err := client.HasFirstUser(ctx)
	if err != nil {
		return xerrors.Errorf("check first user: %w", err)
	}

	if !hasFirstUser {
		log.Printf("Creating first admin user: %s", defaultAdminUsername)
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
		log.Println("First admin user created successfully")
	} else {
		log.Println("First user already exists, skipping creation")
	}

	// Login to get a session token.
	log.Println("Logging in as admin")
	loginResp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    defaultAdminEmail,
		Password: defaultAdminPassword,
	})
	if err != nil {
		return xerrors.Errorf("login as admin: %w", err)
	}
	client.SetSessionToken(loginResp.SessionToken)

	// Check if member user already exists.
	memberExists := false
	_, err = client.User(ctx, defaultMemberUsername)
	if err == nil {
		memberExists = true
	} else {
		var sdkErr *codersdk.Error
		if errors.As(err, &sdkErr) {
			if sdkErr.StatusCode() == http.StatusNotFound || sdkErr.StatusCode() == http.StatusBadRequest {
				memberExists = false
			} else {
				return xerrors.Errorf("check member user: %w", err)
			}
		}
	}

	if !memberExists {
		org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
		if err != nil {
			return xerrors.Errorf("get default organization: %w", err)
		}

		log.Printf("Creating member user: %s", defaultMemberUsername)
		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           defaultMemberEmail,
			Username:        defaultMemberUsername,
			Name:            defaultMemberName,
			Password:        defaultAdminPassword,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{org.ID},
		})
		if err != nil {
			return xerrors.Errorf("create member user: %w", err)
		}
		log.Println("Member user created successfully")
	} else {
		log.Println("Member user already exists, skipping creation")
	}

	// Create docker template if it doesn't exist.
	if err := createDockerTemplate(ctx, client); err != nil {
		// Don't fail setup if template creation fails.
		log.Printf("Warning: failed to create docker template: %v", err)
	}

	log.Printf("Setup completed - admin: %s, member: %s", defaultAdminUsername, defaultMemberUsername)
	return nil
}

func createDockerTemplate(ctx context.Context, client *codersdk.Client) error {
	const templateName = "docker"

	// Check if template already exists.
	org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return xerrors.Errorf("get default organization: %w", err)
	}

	_, err = client.TemplateByName(ctx, org.ID, templateName)
	if err == nil {
		log.Println("Docker template already exists, skipping creation")
		return nil
	}

	// Template doesn't exist, create it.
	log.Println("Creating docker template")

	// Copy template to temp directory and run terraform init to generate lock file.
	log.Println("Initializing terraform providers")
	templateDir := filepath.Join("examples", "templates", "docker")
	tempDir, err := prepareTemplateDir(ctx, templateDir)
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
	log.Println("Uploading template files")
	uploadResp, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(tarData))
	if err != nil {
		return xerrors.Errorf("upload template files: %w", err)
	}

	// Create a template version.
	log.Println("Creating template version")
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
	log.Println("Waiting for template to build")
	version, err = waitForTemplateVersion(ctx, client, version.ID)
	if err != nil {
		return xerrors.Errorf("wait for template version: %w", err)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		log.Printf("Template version build failed: %s", version.Job.Error)
		return xerrors.Errorf("template version failed: %s", version.Job.Status)
	}

	// Create the template.
	log.Println("Finalizing template")
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

	log.Println("Docker template created successfully")
	return nil
}

// prepareTemplateDir copies the template to a temp directory and runs terraform init
// to generate the lock file that Coder's provisioner needs.
func prepareTemplateDir(ctx context.Context, srcDir string) (string, error) {
	// Create temp directory.
	tempDir, err := os.MkdirTemp("", "cdevsetup-template-*")
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

	// Inject additional modules into main.tf for development.
	if err := injectDevModules(filepath.Join(tempDir, "main.tf")); err != nil {
		os.RemoveAll(tempDir)
		return "", xerrors.Errorf("inject dev modules: %w", err)
	}

	// Run terraform init to download providers and create lock file.
	log.Printf("Running terraform init in %s", tempDir)

	cmd := exec.CommandContext(ctx, "terraform", "init")
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tempDir)
		return "", xerrors.Errorf("terraform init failed: %w\nOutput: %s", err, string(output))
	}

	log.Println("Terraform init completed")

	// Remove the .terraform directory - we only need the lock file.
	// The provisioner will download the providers itself.
	tfDir := filepath.Join(tempDir, ".terraform")
	if err := os.RemoveAll(tfDir); err != nil {
		log.Printf("Warning: failed to remove .terraform directory: %v", err)
	}

	return tempDir, nil
}

// injectDevModules appends additional Terraform modules to main.tf for development.
func injectDevModules(mainTFPath string) error {
	const filebrowserModule = `
# ============================================================
# Development modules injected by cdevsetup
# ============================================================

# See https://registry.coder.com/modules/coder/filebrowser
module "filebrowser" {
  count      = data.coder_workspace.me.start_count
  source     = "registry.coder.com/coder/filebrowser/coder"
  version    = "~> 1.0"
  agent_id   = coder_agent.main.id
  agent_name = "main"
}
`

	f, err := os.OpenFile(mainTFPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return xerrors.Errorf("open main.tf: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(filebrowserModule); err != nil {
		return xerrors.Errorf("write filebrowser module: %w", err)
	}

	return nil
}

func waitForTemplateVersion(ctx context.Context, client *codersdk.Client, versionID uuid.UUID) (codersdk.TemplateVersion, error) {
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
				log.Printf("Warning: failed to get template version: %v", err)
				continue
			}

			if !version.Job.Status.Active() {
				return version, nil
			}

			log.Printf("Template version still building: %s", version.Job.Status)
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
