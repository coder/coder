package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisioner/terraform/tfparse"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplatePush(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example")
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatchContext(ctx, m.match)
			pty.WriteLine(m.write)
		}

		w.RequireSuccess()

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
		require.Equal(t, "example", templateVersions[1].Name)
	})

	t.Run("Message less than or equal to 72 chars", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		wantMessage := strings.Repeat("a", 72)

		inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", "example", "--message", wantMessage, "--yes")
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		pty.ExpectNoMatchBefore(ctx, "Template message is longer than 72 characters", "Updated version at")

		w.RequireSuccess()

		// Assert that the template version changed.
		ctx = testutil.Context(t, testutil.WaitMedium)
		templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
		require.Equal(t, wantMessage, templateVersions[1].Message)
	})

	t.Run("Message too long, warn but continue", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		for i, tt := range []struct {
			wantMessage string
			wantMatch   string
		}{
			{wantMessage: strings.Repeat("a", 73), wantMatch: "Template message is longer than 72 characters"},
			{wantMessage: "This is my title\n\nAnd this is my body.", wantMatch: "Template message contains newlines"},
		} {
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--message", tt.wantMessage,
				"--yes",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			pty.ExpectMatchContext(ctx, tt.wantMatch)

			w.RequireSuccess()

			// Assert that the template version changed.
			ctx = testutil.Context(t, testutil.WaitMedium)
			templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2+i)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1+i].ID)
			require.Equal(t, tt.wantMessage, templateVersions[1+i].Message)
		}
	})

	t.Run("NoLockfile", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))

		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "No .terraform.lock.hcl file found"},
			{match: "Upload", write: "no"},
		}
		for _, m := range matches {
			pty.ExpectMatchContext(ctx, m.match)
			if m.write != "" {
				pty.WriteLine(m.write)
			}
		}

		// cmd should error once we say no.
		w.RequireError()
	})

	t.Run("NoLockfileIgnored", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, os.Remove(filepath.Join(source, ".terraform.lock.hcl")))

		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
			"--ignore-lockfile",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		{
			ctx := testutil.Context(t, testutil.WaitMedium)

			pty.ExpectNoMatchBefore(ctx, "No .terraform.lock.hcl file found", "Upload")
			pty.WriteLine("no")
		}

		// cmd should error once we say no.
		w.RequireError()
	})

	t.Run("PushInactiveTemplateVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		inv, root := clitest.New(t, "templates", "push", template.Name,
			"--activate=false",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--name", "example",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatchContext(ctx, m.match)
			pty.WriteLine(m.write)
		}

		w.RequireSuccess()

		// Assert that the template version didn't change.
		ctx = testutil.Context(t, testutil.WaitMedium)
		templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.Equal(t, template.ActiveVersionID, templateVersions[0].ID)
		require.NotEqual(t, "example", templateVersions[0].Name)
	})

	t.Run("UseWorkingDir", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip(`On Windows this test flakes with: "The process cannot access the file because it is being used by another process"`)
		}

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		// Test the cli command.
		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID,
			func(r *codersdk.CreateTemplateRequest) {
				r.Name = filepath.Base(source)
			})

		// Don't pass the name of the template, it should use the
		// directory of the source.
		inv, root := clitest.New(t, "templates", "push",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--test.workdir", source,
			"--force-tty",
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		w := clitest.StartWithWaiter(t, inv)

		matches := []struct {
			match string
			write string
		}{
			{match: "Upload", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatchContext(ctx, m.match)
			pty.WriteLine(m.write)
		}

		w.RequireSuccess()

		// Assert that the template version changed.
		ctx = testutil.Context(t, testutil.WaitMedium)
		templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
	})

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		source, err := echo.Tar(&echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})
		require.NoError(t, err)

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		inv, root := clitest.New(
			t, "templates", "push", "--directory", "-",
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			template.Name,
		)
		clitest.SetupConfig(t, templateAdmin, root)
		pty := ptytest.New(t)
		inv.Stdin = bytes.NewReader(source)
		inv.Stdout = pty.Output()

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		require.NoError(t, <-execDone)

		// Assert that the template version changed.
		templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
			TemplateID: template.ID,
		})
		require.NoError(t, err)
		assert.Len(t, templateVersions, 2)
		assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
	})

	t.Run("ProvisionerTags", func(t *testing.T) {
		t.Parallel()

		t.Run("WorkspaceTagsTerraform", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name         string
				setupDaemon  func(ctx context.Context, store database.Store, owner codersdk.CreateFirstUserResponse, tags database.StringMap, now time.Time) error
				expectOutput string
			}{
				{
					name: "no provisioners available",
					setupDaemon: func(_ context.Context, _ database.Store, _ codersdk.CreateFirstUserResponse, _ database.StringMap, _ time.Time) error {
						return nil
					},
					expectOutput: "there are no provisioners that accept the required tags",
				},
				{
					name: "provisioner stale",
					setupDaemon: func(ctx context.Context, store database.Store, owner codersdk.CreateFirstUserResponse, tags database.StringMap, now time.Time) error {
						pk, err := store.InsertProvisionerKey(ctx, database.InsertProvisionerKeyParams{
							ID:             uuid.New(),
							CreatedAt:      now,
							OrganizationID: owner.OrganizationID,
							Name:           "test",
							Tags:           tags,
							HashedSecret:   []byte("secret"),
						})
						if err != nil {
							return err
						}
						oneHourAgo := now.Add(-time.Hour)
						_, err = store.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
							Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
							LastSeenAt:     sql.NullTime{Time: oneHourAgo, Valid: true},
							CreatedAt:      oneHourAgo,
							Name:           "test",
							Tags:           tags,
							OrganizationID: owner.OrganizationID,
							KeyID:          pk.ID,
						})
						return err
					},
					expectOutput: "Provisioners that accept the required tags have not responded for longer than expected",
				},
				{
					name: "active provisioner",
					setupDaemon: func(ctx context.Context, store database.Store, owner codersdk.CreateFirstUserResponse, tags database.StringMap, now time.Time) error {
						pk, err := store.InsertProvisionerKey(ctx, database.InsertProvisionerKeyParams{
							ID:             uuid.New(),
							CreatedAt:      now,
							OrganizationID: owner.OrganizationID,
							Name:           "test",
							Tags:           tags,
							HashedSecret:   []byte("secret"),
						})
						if err != nil {
							return err
						}
						_, err = store.UpsertProvisionerDaemon(ctx, database.UpsertProvisionerDaemonParams{
							Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
							LastSeenAt:     sql.NullTime{Time: now, Valid: true},
							CreatedAt:      now,
							Name:           "test-active",
							Tags:           tags,
							OrganizationID: owner.OrganizationID,
							KeyID:          pk.ID,
						})
						return err
					},
					expectOutput: "",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					// Start an instance **without** a built-in provisioner.
					// We're not actually testing that the Terraform applies.
					// What we test is that a provisioner job is created with the expected
					// tags based on the __content__ of the Terraform.
					store, ps := dbtestutil.NewDB(t)
					client := coderdtest.New(t, &coderdtest.Options{
						Database: store,
						Pubsub:   ps,
					})

					owner := coderdtest.CreateFirstUser(t, client)
					templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

					// Create a tar file with some pre-defined content
					tarFile := testutil.CreateTar(t, map[string]string{
						"main.tf": `
							variable "a" {
								type = string
								default = "1"
							}
							data "coder_parameter" "b" {
								name = "b"
								type = string
								default = "2"
							}
							resource "null_resource" "test" {}
							data "coder_workspace_tags" "tags" {
								tags = {
									"a": var.a,
									"b": data.coder_parameter.b.value,
									"test_name": "` + tt.name + `"
								}
							}`,
					})

					// Write the tar file to disk.
					tempDir := t.TempDir()
					err := tfparse.WriteArchive(tarFile, "application/x-tar", tempDir)
					require.NoError(t, err)

					wantTags := database.StringMap(provisionersdk.MutateTags(uuid.Nil, map[string]string{
						"a":         "1",
						"b":         "2",
						"test_name": tt.name,
					}))

					templateName := testutil.GetRandomNameHyphenated(t)

					inv, root := clitest.New(t, "templates", "push", templateName, "-d", tempDir, "--yes")
					clitest.SetupConfig(t, templateAdmin, root)
					pty := ptytest.New(t).Attach(inv)

					setupCtx := testutil.Context(t, testutil.WaitMedium)
					now := dbtime.Now()
					require.NoError(t, tt.setupDaemon(setupCtx, store, owner, wantTags, now))

					ctx := testutil.Context(t, testutil.WaitMedium)
					cancelCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					inv = inv.WithContext(cancelCtx)
					w := clitest.StartWithWaiter(t, inv)

					require.Eventually(t, func() bool {
						jobs, err := store.GetProvisionerJobsCreatedAfter(ctx, time.Time{})
						if !assert.NoError(t, err) {
							return false
						}
						if len(jobs) == 0 {
							return false
						}
						return assert.EqualValues(t, wantTags, jobs[0].Tags)
					}, testutil.WaitShort, testutil.IntervalFast)

					if tt.expectOutput != "" {
						pty.ExpectMatchContext(ctx, tt.expectOutput)
					}

					cancel()
					_ = w.Wait()
				})
			}
		})

		t.Run("ChangeTags", func(t *testing.T) {
			t.Parallel()

			// Start the first provisioner
			client, provisionerDocker, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				ProvisionerDaemonTags: map[string]string{
					"docker": "true",
				},
			})
			defer provisionerDocker.Close()

			// Start the second provisioner
			provisionerFoobar := coderdtest.NewTaggedProvisionerDaemon(t, api, "provisioner-foobar", map[string]string{
				"foobar": "foobaz",
			})
			defer provisionerFoobar.Close()

			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			// Create the template with initial tagged template version.
			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
				ctvr.ProvisionerTags = map[string]string{
					"docker": "true",
				}
			})
			templateVersion = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Push new template version with different provisioner tags.
			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: echo.ApplyComplete,
			})
			inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", template.Name,
				"--provisioner-tag", "foobar=foobaz")
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Verify template version tags
			template, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)

			templateVersion, err = client.TemplateVersion(context.Background(), template.ActiveVersionID)
			require.NoError(t, err)
			require.EqualValues(t, map[string]string{"foobar": "foobaz", "owner": "", "scope": "organization"}, templateVersion.Job.Tags)
		})

		t.Run("DeleteTags", func(t *testing.T) {
			t.Parallel()

			// Start the first provisioner with no tags.
			client, provisionerDocker, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				ProvisionerDaemonTags:    map[string]string{},
			})
			defer provisionerDocker.Close()

			// Start the second provisioner with a tag set.
			provisionerFoobar := coderdtest.NewTaggedProvisionerDaemon(t, api, "provisioner-foobar", map[string]string{
				"foobar": "foobaz",
			})
			defer provisionerFoobar.Close()

			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			// Create the template with initial tagged template version.
			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
				ctvr.ProvisionerTags = map[string]string{
					"foobar": "foobaz",
				}
			})
			templateVersion = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Stop the tagged provisioner daemon.
			provisionerFoobar.Close()

			// Push new template version with no provisioner tags.
			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: echo.ApplyComplete,
			})
			inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", template.Name, "--provisioner-tag=\"-\"")
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Verify template version tags
			template, err := client.Template(ctx, template.ID)
			require.NoError(t, err)

			templateVersion, err = client.TemplateVersion(ctx, template.ActiveVersionID)
			require.NoError(t, err)
			require.EqualValues(t, map[string]string{"owner": "", "scope": "organization"}, templateVersion.Job.Tags)
		})

		t.Run("DoNotChangeTags", func(t *testing.T) {
			t.Parallel()

			// Start the tagged provisioner
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				ProvisionerDaemonTags: map[string]string{
					"docker": "true",
				},
			})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			// Create the template with initial tagged template version.
			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
				ctvr.ProvisionerTags = map[string]string{
					"docker": "true",
				}
			})
			templateVersion = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Push new template version without provisioner tags. CLI should reuse tags from the previous version.
			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: echo.ApplyComplete,
			})
			inv, root := clitest.New(t, "templates", "push", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho), "--name", template.Name)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Verify template version tags
			template, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)

			templateVersion, err = client.TemplateVersion(context.Background(), template.ActiveVersionID)
			require.NoError(t, err)
			require.EqualValues(t, map[string]string{"docker": "true", "owner": "", "scope": "organization"}, templateVersion.Job.Tags)
		})
	})

	t.Run("Variables", func(t *testing.T) {
		t.Parallel()

		initialTemplateVariables := []*proto.TemplateVariable{
			{
				Name:         "first_variable",
				Description:  "This is the first variable",
				Type:         "string",
				DefaultValue: "abc",
				Required:     false,
				Sensitive:    true,
			},
		}

		t.Run("VariableIsRequired", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			//nolint:gocritic
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable.",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			tempDir := t.TempDir()
			removeTmpDirUntilSuccessAfterTest(t, tempDir)
			variablesFile, _ := os.CreateTemp(tempDir, "variables*.yaml")
			_, _ = variablesFile.WriteString(`second_variable: foobar`)
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
				"--variables-file", variablesFile.Name(),
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "foobar", templateVariables[1].Value)
		})

		t.Run("VariableIsOptionalButNotProvided", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			//nolint:gocritic
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:         "second_variable",
					Description:  "This is the second variable",
					Type:         "string",
					DefaultValue: "abc",
					Required:     true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "abc", templateVariables[1].Value)
			require.Equal(t, templateVariables[1].DefaultValue, templateVariables[1].Value)
		})

		t.Run("WithVariableOption", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVersion := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, createEchoResponsesWithTemplateVariables(initialTemplateVariables))
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, templateVersion.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, templateVersion.ID)

			// Test the cli command.
			//nolint:gocritic
			modifiedTemplateVariables := append(initialTemplateVariables,
				&proto.TemplateVariable{
					Name:        "second_variable",
					Description: "This is the second variable.",
					Type:        "string",
					Required:    true,
				},
			)
			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(modifiedTemplateVariables))
			inv, root := clitest.New(t,
				"templates", "push", template.Name,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--name", "example",
				"--variable", "second_variable=foobar",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t)
			inv.Stdin = pty.Input()
			inv.Stdout = pty.Output()

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				pty.WriteLine(m.write)
			}

			w.RequireSuccess()

			// Assert that the template version changed.
			templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			assert.Len(t, templateVersions, 2)
			assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
			require.Equal(t, "example", templateVersions[1].Name)

			templateVariables, err := client.TemplateVersionVariables(context.Background(), templateVersions[1].ID)
			require.NoError(t, err)
			assert.Len(t, templateVariables, 2)
			require.Equal(t, "second_variable", templateVariables[1].Name)
			require.Equal(t, "foobar", templateVariables[1].Value)
		})

		t.Run("CreateTemplate", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			source := clitest.CreateTemplateVersionSource(t, completeWithAgent())

			const templateName = "my-template"
			args := []string{
				"templates",
				"push",
				templateName,
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
			}
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			matches := []struct {
				match string
				write string
			}{
				{match: "Upload", write: "yes"},
				{match: "template has been created"},
			}
			for _, m := range matches {
				pty.ExpectMatchContext(ctx, m.match)
				if m.write != "" {
					pty.WriteLine(m.write)
				}
			}

			w.RequireSuccess()

			template, err := client.TemplateByName(context.Background(), owner.OrganizationID, templateName)
			require.NoError(t, err)
			require.Equal(t, templateName, template.Name)
			require.NotEqual(t, uuid.Nil, template.ActiveVersionID)
		})

		t.Run("NoStdinWithCurrentDirectory", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: echo.ApplyComplete,
			})

			inv, root := clitest.New(t, "templates", "push", template.Name,
				"--directory", ".",
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--test.workdir", source,
				"--name", "example",
				"--yes")
			clitest.SetupConfig(t, templateAdmin, root)

			inv.Stdin = strings.NewReader("invalid tar content that would cause failure")

			ctx := testutil.Context(t, testutil.WaitMedium)
			err := inv.WithContext(ctx).Run()
			require.NoError(t, err, "Should succeed without reading from stdin")

			templateVersions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			require.NoError(t, err)
			require.Len(t, templateVersions, 2)
			require.Equal(t, "example", templateVersions[1].Name)
		})

		t.Run("PromptForDifferentRequiredTypes", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVariables := []*proto.TemplateVariable{
				{
					Name:        "string_var",
					Description: "A string variable",
					Type:        "string",
					Required:    true,
				},
				{
					Name:        "number_var",
					Description: "A number variable",
					Type:        "number",
					Required:    true,
				},
				{
					Name:        "bool_var",
					Description: "A boolean variable",
					Type:        "bool",
					Required:    true,
				},
				{
					Name:        "sensitive_var",
					Description: "A sensitive variable",
					Type:        "string",
					Required:    true,
					Sensitive:   true,
				},
			}

			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(templateVariables))
			inv, root := clitest.New(t, "templates", "push", "test-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			// Select "Yes" for the "Upload <template_path>" prompt
			pty.ExpectMatchContext(ctx, "Upload")
			pty.WriteLine("yes")

			// Variables are prompted in alphabetical order.
			// Boolean variable automatically selects the first option ("true")
			pty.ExpectMatchContext(ctx, "var.bool_var")

			pty.ExpectMatchContext(ctx, "var.number_var")
			pty.ExpectMatchContext(ctx, "Enter value:")
			pty.WriteLine("42")

			pty.ExpectMatchContext(ctx, "var.sensitive_var")
			pty.ExpectMatchContext(ctx, "Enter value:")
			pty.WriteLine("secret-value")

			pty.ExpectMatchContext(ctx, "var.string_var")
			pty.ExpectMatchContext(ctx, "Enter value:")
			pty.WriteLine("test-string")

			w.RequireSuccess()
		})

		t.Run("ValidateNumberInput", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVariables := []*proto.TemplateVariable{
				{
					Name:        "number_var",
					Description: "A number that requires validation",
					Type:        "number",
					Required:    true,
				},
			}

			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(templateVariables))
			inv, root := clitest.New(t, "templates", "push", "test-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			// Select "Yes" for the "Upload <template_path>" prompt
			pty.ExpectMatchContext(ctx, "Upload")
			pty.WriteLine("yes")

			pty.ExpectMatchContext(ctx, "var.number_var")

			pty.WriteLine("not-a-number")
			pty.ExpectMatchContext(ctx, "must be a valid number")

			pty.WriteLine("123.45")

			w.RequireSuccess()
		})

		t.Run("DontPromptForDefaultValues", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVariables := []*proto.TemplateVariable{
				{
					Name:         "with_default",
					Type:         "string",
					Required:     true,
					DefaultValue: "default-value",
				},
				{
					Name:     "without_default",
					Type:     "string",
					Required: true,
				},
			}

			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(templateVariables))
			inv, root := clitest.New(t, "templates", "push", "test-template", "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			// Select "Yes" for the "Upload <template_path>" prompt
			pty.ExpectMatchContext(ctx, "Upload")
			pty.WriteLine("yes")

			pty.ExpectMatchContext(ctx, "var.without_default")
			pty.WriteLine("test-value")

			w.RequireSuccess()
		})

		t.Run("VariableSourcesPriority", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			templateVariables := []*proto.TemplateVariable{
				{
					Name:        "cli_flag_var",
					Description: "Variable provided via CLI flag",
					Type:        "string",
					Required:    true,
				},
				{
					Name:        "file_var",
					Description: "Variable provided via file",
					Type:        "string",
					Required:    true,
				},
				{
					Name:        "prompt_var",
					Description: "Variable provided via prompt",
					Type:        "string",
					Required:    true,
				},
				{
					Name:        "cli_overrides_file_var",
					Description: "Variable in both CLI and file",
					Type:        "string",
					Required:    true,
				},
			}

			source := clitest.CreateTemplateVersionSource(t, createEchoResponsesWithTemplateVariables(templateVariables))

			// Create a temporary variables file.
			tempDir := t.TempDir()
			removeTmpDirUntilSuccessAfterTest(t, tempDir)
			variablesFile, err := os.CreateTemp(tempDir, "variables*.yaml")
			require.NoError(t, err)
			_, err = variablesFile.WriteString(`file_var: from-file
cli_overrides_file_var: from-file`)
			require.NoError(t, err)
			require.NoError(t, variablesFile.Close())

			inv, root := clitest.New(t, "templates", "push", "test-template",
				"--directory", source,
				"--test.provisioner", string(database.ProvisionerTypeEcho),
				"--variables-file", variablesFile.Name(),
				"--variable", "cli_flag_var=from-cli-flag",
				"--variable", "cli_overrides_file_var=from-cli-override",
			)
			clitest.SetupConfig(t, templateAdmin, root)
			pty := ptytest.New(t).Attach(inv)

			ctx := testutil.Context(t, testutil.WaitMedium)
			inv = inv.WithContext(ctx)
			w := clitest.StartWithWaiter(t, inv)

			// Select "Yes" for the "Upload <template_path>" prompt
			pty.ExpectMatchContext(ctx, "Upload")
			pty.WriteLine("yes")

			// Only check for prompt_var, other variables should not prompt
			pty.ExpectMatchContext(ctx, "var.prompt_var")
			pty.ExpectMatchContext(ctx, "Enter value:")
			pty.WriteLine("from-prompt")

			w.RequireSuccess()

			template, err := client.TemplateByName(context.Background(), owner.OrganizationID, "test-template")
			require.NoError(t, err)

			templateVersionVars, err := client.TemplateVersionVariables(context.Background(), template.ActiveVersionID)
			require.NoError(t, err)
			require.Len(t, templateVersionVars, 4)

			varMap := make(map[string]string)
			for _, tv := range templateVersionVars {
				varMap[tv.Name] = tv.Value
			}

			require.Equal(t, "from-cli-flag", varMap["cli_flag_var"])
			require.Equal(t, "from-file", varMap["file_var"])
			require.Equal(t, "from-prompt", varMap["prompt_var"])
			require.Equal(t, "from-cli-override", varMap["cli_overrides_file_var"])
		})
	})
}

func createEchoResponsesWithTemplateVariables(templateVariables []*proto.TemplateVariable) *echo.Responses {
	return &echo.Responses{
		Parse: []*proto.Response{
			{
				Type: &proto.Response_Parse{
					Parse: &proto.ParseComplete{
						TemplateVariables: templateVariables,
					},
				},
			},
		},
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
	}
}

func completeWithAgent() *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionPlan: []*proto.Response{
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "smith",
										OperatingSystem: "linux",
										Architecture:    "i386",
									},
								},
							},
						},
					},
				},
			},
		},
		ProvisionApply: []*proto.Response{
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{
							{
								Type: "compute",
								Name: "main",
								Agents: []*proto.Agent{
									{
										Name:            "smith",
										OperatingSystem: "linux",
										Architecture:    "i386",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Need this for Windows because of a known issue with Go:
// https://github.com/golang/go/issues/52986
func removeTmpDirUntilSuccessAfterTest(t *testing.T, tempDir string) {
	t.Helper()
	t.Cleanup(func() {
		err := os.RemoveAll(tempDir)
		for err != nil {
			err = os.RemoveAll(tempDir)
		}
	})
}
