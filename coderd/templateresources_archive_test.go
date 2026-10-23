package coderd_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func uploadTemplateResourceFiles(t *testing.T, client *codersdk.Client, files map[string]string) uuid.UUID {
	t.Helper()
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	for name, content := range files {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0o755}))
		_, err := writer.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	upload, err := client.Upload(t.Context(), codersdk.ContentTypeTar, bytes.NewReader(data.Bytes()))
	require.NoError(t, err)
	return upload.ID
}

func TestTemplateResourceRevisionArchive(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	admin, adminUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	reader, readerUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID,
		UserACL: database.TemplateACL{
			adminUser.ID.String():  {policy.ActionRead, policy.ActionUpdate},
			readerUser.ID.String(): {policy.ActionRead},
		},
	})
	ctx := t.Context()
	files := map[string]string{
		"resources.tf":          `module "local" { source = "./modules/local" }`,
		"modules/local/main.tf": `output "value" { value = "hello" }`,
		"scripts/start.sh":      "#!/bin/sh\necho hello\n",
		".terraform.lock.hcl":   "# dependency selections\n",
	}
	uploadID := uploadTemplateResourceFiles(t, client, files)
	revision, err := client.CreateTemplateResourceRevision(ctx, template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: uploadID})
	require.NoError(t, err)
	require.Empty(t, revision.Source, "main.tf is not required")
	require.NotEqual(t, uuid.Nil, revision.FileID)
	require.NotEqual(t, uploadID, revision.FileID, "snapshot normalized metadata and implicit directories")

	// A second template ACL admin can open the immutable archive before Build.
	detail, err := admin.TemplateResources(ctx, template.ID)
	require.NoError(t, err)
	require.Empty(t, detail.Runs)
	require.Equal(t, revision.FileID, detail.Revisions[0].FileID)
	data, contentType, err := admin.Download(ctx, revision.FileID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ContentTypeTar, contentType)
	archive := tar.NewReader(bytes.NewReader(data))
	contents := make(map[string]string)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if header.Typeflag == tar.TypeDir {
			continue
		}
		content, err := io.ReadAll(archive)
		require.NoError(t, err)
		contents[header.Name] = string(content)
	}
	require.Equal(t, files, contents)

	for _, id := range []uuid.UUID{revision.FileID, uploadID} {
		_, _, err = reader.Download(ctx, id)
		require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode(), "read-only template access does not grant archive access")
	}
	_, _, err = admin.Download(ctx, uploadID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode(), "the original unrelated upload is not authorized by the normalized revision")
	unrelated := uploadTemplateResourceFiles(t, client, map[string]string{"main.tf": "# unrelated upload"})
	_, _, err = admin.Download(ctx, unrelated)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = admin.CreateTemplateResourceRevision(ctx, template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: unrelated})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode(), "revision creation must authorize uploaded file reads")

	privateTemplate := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	private, err := client.CreateTemplateResourceRevision(ctx, privateTemplate.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# private revision"})
	require.NoError(t, err)
	_, _, err = admin.Download(ctx, private.FileID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode(), "an archive associated with a different template is not authorized")

	// Reusing an authorized revision archive still creates an independent revision.
	newer, err := admin.CreateTemplateResourceRevision(ctx, template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: revision.FileID})
	require.NoError(t, err)
	require.NotEqual(t, revision.ID, newer.ID)
	run, err := admin.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "plan"})
	require.NoError(t, err)
	//nolint:gocritic // Verify the native plan receives the complete selected snapshot.
	job, err := db.GetProvisionerJobByID(dbauthz.AsSystemRestricted(ctx), run.Job.ID)
	require.NoError(t, err)
	require.Equal(t, revision.FileID, job.FileID)
	after, _, err := admin.Download(ctx, revision.FileID)
	require.NoError(t, err)
	require.Equal(t, data, after, "saving and building never mutate an older archive")
}

func TestTemplateResourceRevisionArchiveValidation(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"second_file_backend", map[string]string{"main.tf": "# safe preview", "backend.tf": "terraform {\n backend \"s3\" {}\n}"}},
		{"json_backend", map[string]string{"main.tf": "# safe preview", "backend.tf.json": `{"terraform":{"backend":{"s3":{}}}}`}},
		{"json_cloud", map[string]string{"resources.tf.json": `{"terraform":{"cloud":{}}}`}},
		{"malformed", map[string]string{"resources.tf": "resource {"}},
		{"no_root", map[string]string{"modules/local/main.tf": "# module"}},
		{"traversal", map[string]string{"main.tf": "# root", "../script": ""}},
		{"local_state", map[string]string{"main.tf": "# root", "./terraform.tfstate": `{"version":4,"serial":1,"lineage":"foreign","outputs":{},"resources":[]}`}},
		{"local_state_backup", map[string]string{"main.tf": "# root", "terraform.tfstate.backup": "{}"}},
		{"backend_metadata", map[string]string{"main.tf": "# root", ".terraform/terraform.tfstate": `{"backend":{"type":"s3"}}`}},
		{"workspace_state", map[string]string{"main.tf": "# root", "terraform.tfstate.d/foreign/terraform.tfstate": "{}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fileID := uploadTemplateResourceFiles(t, client, tc.files)
			_, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: fileID})
			require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
		})
	}
	for _, symlink := range []bool{false, true} {
		name := "zip_modules"
		if symlink {
			name = "zip_symlink_rejected"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var data bytes.Buffer
			writer := zip.NewWriter(&data)
			for _, name := range []string{"main.tf", "modules/", "modules/local.tf"} {
				header := &zip.FileHeader{Name: name}
				header.SetMode(0o600)
				content := "# Terraform configuration"
				if name == "modules/" {
					header.SetMode(fs.ModeDir | 0o700)
					content = ""
				} else if name == "modules/local.tf" && symlink {
					header.SetMode(fs.ModeSymlink | 0o777)
					content = "../main.tf"
				}
				file, err := writer.CreateHeader(header)
				require.NoError(t, err)
				_, err = io.WriteString(file, content)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			upload, err := client.Upload(t.Context(), codersdk.ContentTypeZip, bytes.NewReader(data.Bytes()))
			require.NoError(t, err)
			_, err = client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: upload.ID})
			if symlink {
				require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
			} else {
				require.NoError(t, err)
			}
		})
	}
	t.Run("request_contract", func(t *testing.T) {
		t.Parallel()
		fileID := uploadTemplateResourceFiles(t, client, map[string]string{"main.tf": "# root"})
		for _, req := range []codersdk.CreateTemplateResourceRevisionRequest{
			{}, {Source: " "}, {FileID: fileID, Source: "# root"}, {FileID: fileID, Source: " "},
		} {
			_, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, req)
			require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
		}
		_, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: uuid.New()})
		require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
		revision, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{FileID: fileID})
		require.NoError(t, err)
		require.Equal(t, "# root", revision.Source)
	})
}
