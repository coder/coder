package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateEdit(t *testing.T) {
	t.Parallel()

	t.Run("FirstEmptyThenModified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		name := "new-template-name"
		displayName := "New Display Name 789"
		desc := "lorem ipsum dolor sit amet et cetera"
		icon := "/icon/new-icon.png"
		defaultTTL := 12 * time.Hour
		allowUserCancelWorkspaceJobs := false

		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--display-name", displayName,
			"--description", desc,
			"--icon", icon,
			"--default-ttl", defaultTTL.String(),
			"--allow-user-cancel-workspace-jobs=" + strconv.FormatBool(allowUserCancelWorkspaceJobs),
		}
		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, templateAdmin, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, name, updated.Name)
		assert.Equal(t, displayName, updated.DisplayName)
		assert.Equal(t, desc, updated.Description)
		assert.Equal(t, icon, updated.Icon)
		assert.Equal(t, defaultTTL.Milliseconds(), updated.DefaultTTLMillis)
		assert.Equal(t, allowUserCancelWorkspaceJobs, updated.AllowUserCancelWorkspaceJobs)
	})
	t.Run("FirstEmptyThenNotModified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", template.Name,
			"--description", template.Description,
			"--icon", template.Icon,
			"--default-ttl", (time.Duration(template.DefaultTTLMillis) * time.Millisecond).String(),
			"--allow-user-cancel-workspace-jobs=" + strconv.FormatBool(template.AllowUserCancelWorkspaceJobs),
		}
		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, templateAdmin, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()

		require.ErrorContains(t, err, "not modified")

		// Assert that the template metadata did not change.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
		assert.Equal(t, template.AllowUserCancelWorkspaceJobs, updated.AllowUserCancelWorkspaceJobs)
	})
	t.Run("InvalidDisplayName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", template.Name,
			"--display-name", " a-b-c",
		}
		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, templateAdmin, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()

		require.Error(t, err, "client call must fail")
		_, isSdkError := codersdk.AsError(err)
		require.True(t, isSdkError, "sdk error is expected")

		// Assert that the template metadata did not change.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, "", template.DisplayName)
	})
	t.Run("WithPropertiesThenModified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		initialDisplayName := "This is a template"
		initialDescription := "This is description"
		initialIcon := "/img/icon.png"

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DisplayName = initialDisplayName
			ctr.Description = initialDescription
			ctr.Icon = initialIcon
		})

		// Test created template
		created, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, initialDisplayName, created.DisplayName)
		assert.Equal(t, initialDescription, created.Description)
		assert.Equal(t, initialIcon, created.Icon)

		// Test the cli command.
		displayName := "New Display Name 789"
		description := "New Description ABC"
		icon := "/icon/new-icon.png"
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--description", description,
			"--display-name", displayName,
			"--icon", icon,
		}
		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, templateAdmin, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err = inv.WithContext(ctx).Run()

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name) // doesn't change
		assert.Equal(t, description, updated.Description)
		assert.Equal(t, displayName, updated.DisplayName)
		assert.Equal(t, icon, updated.Icon)
	})
	t.Run("WithPropertiesThenEmptyEdit", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		initialDisplayName := "This is a template"
		initialDescription := "This is description"
		initialIcon := "/img/icon.png"

		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DisplayName = initialDisplayName
			ctr.Description = initialDescription
			ctr.Icon = initialIcon
		})

		// Test created template
		created, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, initialDisplayName, created.DisplayName)
		assert.Equal(t, initialDescription, created.Description)
		assert.Equal(t, initialIcon, created.Icon)

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
		}
		inv, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, templateAdmin, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err = inv.WithContext(ctx).Run()

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		// Properties don't change
		assert.Equal(t, template.Name, updated.Name)
		// These properties are removed, as the API considers it as "delete" request
		// See: https://github.com/coder/coder/issues/5066
		assert.Equal(t, "", updated.Description)
		assert.Equal(t, "", updated.Icon)
		assert.Equal(t, "", updated.DisplayName)
	})
	t.Run("Autostop/startRequirement", func(t *testing.T) {
		t.Parallel()
		t.Run("BlockedAGPL", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.AutostopRequirement = nil
			})

			cases := []struct {
				name  string
				flags []string
				ok    bool
			}{
				{
					name: "Weekdays",
					flags: []string{
						"--autostop-requirement-weekdays", "monday",
					},
				},
				{
					name: "WeekdaysNoneAllowed",
					flags: []string{
						"--autostop-requirement-weekdays", "none",
					},
					ok: true,
				},
				{
					name: "Weeks",
					flags: []string{
						"--autostop-requirement-weeks", "1",
					},
				},
				{
					name: "AutostartDays",
					flags: []string{
						"--autostart-requirement-weekdays", "monday",
					},
				},
			}

			for _, c := range cases {
				c := c
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()

					cmdArgs := []string{
						"templates",
						"edit",
						template.Name,
					}
					cmdArgs = append(cmdArgs, c.flags...)
					inv, root := clitest.New(t, cmdArgs...)
					clitest.SetupConfig(t, templateAdmin, root)

					ctx := testutil.Context(t, testutil.WaitLong)
					err := inv.WithContext(ctx).Run()
					if c.ok {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.ErrorContains(t, err, "appears to be an AGPL deployment")
					}

					// Assert that the template metadata did not change.
					updated, err := client.Template(context.Background(), template.ID)
					require.NoError(t, err)
					assert.Equal(t, template.Name, updated.Name)
					assert.Equal(t, template.Description, updated.Description)
					assert.Equal(t, template.Icon, updated.Icon)
					assert.Equal(t, template.DisplayName, updated.DisplayName)
					assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
					assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
					assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
					assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
					assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
				})
			}
		})

		t.Run("BlockedNotEntitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.AutostopRequirement = nil
			})

			// Make a proxy server that will return a valid entitlements
			// response, but without advanced scheduling entitlement.
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     false,
							Limit:       nil,
							Actual:      nil,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			t.Cleanup(proxy.Close)

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			cases := []struct {
				name  string
				flags []string
				ok    bool
			}{
				{
					name: "Weekdays",
					flags: []string{
						"--autostop-requirement-weekdays", "monday",
					},
				},
				{
					name: "WeekdaysNoneAllowed",
					flags: []string{
						"--autostop-requirement-weekdays", "none",
					},
					ok: true,
				},
				{
					name: "Weeks",
					flags: []string{
						"--autostop-requirement-weeks", "1",
					},
				},
			}

			for _, c := range cases {
				c := c
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()

					cmdArgs := []string{
						"templates",
						"edit",
						template.Name,
					}
					cmdArgs = append(cmdArgs, c.flags...)
					inv, root := clitest.New(t, cmdArgs...)
					clitest.SetupConfig(t, proxyClient, root)

					ctx := testutil.Context(t, testutil.WaitLong)
					err := inv.WithContext(ctx).Run()
					if c.ok {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.ErrorContains(t, err, "license is not entitled")
					}

					// Assert that the template metadata did not change.
					updated, err := client.Template(context.Background(), template.ID)
					require.NoError(t, err)
					assert.Equal(t, template.Name, updated.Name)
					assert.Equal(t, template.Description, updated.Description)
					assert.Equal(t, template.Icon, updated.Icon)
					assert.Equal(t, template.DisplayName, updated.DisplayName)
					assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
					assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
					assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
					assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
				})
			}
		})
		t.Run("Entitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.AutostopRequirement = nil
			})

			// Make a proxy server that will return a valid entitlements
			// response, including a valid advanced scheduling entitlement.
			var updateTemplateCalled int64
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						var one int64 = 1
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     true,
							Limit:       &one,
							Actual:      &one,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}
				if strings.HasPrefix(r.URL.Path, "/api/v2/templates/") {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					_ = r.Body.Close()

					var req codersdk.UpdateTemplateMeta
					err = json.Unmarshal(body, &req)
					require.NoError(t, err)
					assert.Equal(t, req.AutostopRequirement.DaysOfWeek, []string{"monday", "tuesday"})
					assert.EqualValues(t, req.AutostopRequirement.Weeks, 3)

					r.Body = io.NopCloser(bytes.NewReader(body))
					atomic.AddInt64(&updateTemplateCalled, 1)
					// We still want to call the real route.
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			defer proxy.Close()

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			// Test the cli command.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--autostop-requirement-weekdays", "monday,tuesday",
				"--autostop-requirement-weeks", "3",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.NoError(t, err)

			require.EqualValues(t, 1, atomic.LoadInt64(&updateTemplateCalled))

			// Assert that the template metadata did not change. We verify the
			// correct request gets sent to the server already.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
			assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
			assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
		})
	})
	// TODO(@dean): remove this test when we remove max_ttl
	t.Run("MaxTTL", func(t *testing.T) {
		t.Parallel()
		t.Run("BlockedAGPL", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.MaxTTLMillis = nil
			})

			// Test the cli command.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--max-ttl", "1h",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, templateAdmin, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err := inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "appears to be an AGPL deployment")

			// Assert that the template metadata did not change.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		})

		t.Run("BlockedNotEntitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.MaxTTLMillis = nil
			})

			// Make a proxy server that will return a valid entitlements
			// response, but without advanced scheduling entitlement.
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     false,
							Limit:       nil,
							Actual:      nil,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			defer proxy.Close()

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			// Test the cli command.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--max-ttl", "1h",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "license is not entitled")

			// Assert that the template metadata did not change.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		})
		t.Run("Entitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.MaxTTLMillis = nil
			})

			// Make a proxy server that will return a valid entitlements
			// response, including a valid advanced scheduling entitlement.
			var updateTemplateCalled int64
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						var one int64 = 1
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     true,
							Limit:       &one,
							Actual:      &one,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}
				if strings.HasPrefix(r.URL.Path, "/api/v2/templates/") {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					_ = r.Body.Close()

					var req codersdk.UpdateTemplateMeta
					err = json.Unmarshal(body, &req)
					require.NoError(t, err)
					assert.Equal(t, time.Hour.Milliseconds(), req.MaxTTLMillis)

					r.Body = io.NopCloser(bytes.NewReader(body))
					atomic.AddInt64(&updateTemplateCalled, 1)
					// We still want to call the real route.
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			defer proxy.Close()

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			// Test the cli command.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--max-ttl", "1h",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.NoError(t, err)

			require.EqualValues(t, 1, atomic.LoadInt64(&updateTemplateCalled))

			// Assert that the template metadata did not change. We verify the
			// correct request gets sent to the server already.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		})
	})
	t.Run("AllowUserScheduling", func(t *testing.T) {
		t.Parallel()
		t.Run("BlockedAGPL", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = nil
				ctr.AutostopRequirement = nil
				ctr.FailureTTLMillis = nil
				ctr.TimeTilDormantMillis = nil
			})

			// Test the cli command with --allow-user-autostart.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--allow-user-autostart=false",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, templateAdmin, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err := inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "appears to be an AGPL deployment")

			// Test the cli command with --allow-user-autostop.
			cmdArgs = []string{
				"templates",
				"edit",
				template.Name,
				"--allow-user-autostop=false",
			}
			inv, root = clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, templateAdmin, root)

			ctx = testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "appears to be an AGPL deployment")

			// Assert that the template metadata did not change.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
			assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
			assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
			assert.Equal(t, template.AllowUserAutostart, updated.AllowUserAutostart)
			assert.Equal(t, template.AllowUserAutostop, updated.AllowUserAutostop)
			assert.Equal(t, template.FailureTTLMillis, updated.FailureTTLMillis)
			assert.Equal(t, template.TimeTilDormantMillis, updated.TimeTilDormantMillis)
		})

		t.Run("BlockedNotEntitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			// Make a proxy server that will return a valid entitlements
			// response, but without advanced scheduling entitlement.
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     false,
							Limit:       nil,
							Actual:      nil,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			defer proxy.Close()

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			// Test the cli command with --allow-user-autostart.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--allow-user-autostart=false",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "license is not entitled")

			// Test the cli command with --allow-user-autostop.
			cmdArgs = []string{
				"templates",
				"edit",
				template.Name,
				"--allow-user-autostop=false",
			}
			inv, root = clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx = testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.Error(t, err)
			require.ErrorContains(t, err, "license is not entitled")

			// Assert that the template metadata did not change.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
			assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
			assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
			assert.Equal(t, template.AllowUserAutostart, updated.AllowUserAutostart)
			assert.Equal(t, template.AllowUserAutostop, updated.AllowUserAutostop)
			assert.Equal(t, template.FailureTTLMillis, updated.FailureTTLMillis)
			assert.Equal(t, template.TimeTilDormantMillis, updated.TimeTilDormantMillis)
		})
		t.Run("Entitled", func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

			// Make a proxy server that will return a valid entitlements
			// response, including a valid advanced scheduling entitlement.
			var updateTemplateCalled int64
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/entitlements" {
					res := codersdk.Entitlements{
						Features:         map[codersdk.FeatureName]codersdk.Feature{},
						Warnings:         []string{},
						Errors:           []string{},
						HasLicense:       true,
						Trial:            true,
						RequireTelemetry: false,
					}
					for _, feature := range codersdk.FeatureNames {
						var one int64 = 1
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     true,
							Limit:       &one,
							Actual:      &one,
						}
					}
					httpapi.Write(r.Context(), w, http.StatusOK, res)
					return
				}
				if strings.HasPrefix(r.URL.Path, "/api/v2/templates/") {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					_ = r.Body.Close()

					var req codersdk.UpdateTemplateMeta
					err = json.Unmarshal(body, &req)
					require.NoError(t, err)
					assert.False(t, req.AllowUserAutostart)
					assert.False(t, req.AllowUserAutostop)

					r.Body = io.NopCloser(bytes.NewReader(body))
					atomic.AddInt64(&updateTemplateCalled, 1)
					// We still want to call the real route.
				}

				// Otherwise, proxy the request to the real API server.
				rp := httputil.NewSingleHostReverseProxy(client.URL)
				tp := &http.Transport{}
				defer tp.CloseIdleConnections()
				rp.Transport = tp
				rp.ServeHTTP(w, r)
			}))
			defer proxy.Close()

			// Create a new client that uses the proxy server.
			proxyURL, err := url.Parse(proxy.URL)
			require.NoError(t, err)
			proxyClient := codersdk.New(proxyURL)
			proxyClient.SetSessionToken(templateAdmin.SessionToken())
			t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

			// Test the cli command.
			cmdArgs := []string{
				"templates",
				"edit",
				template.Name,
				"--allow-user-autostart=false",
				"--allow-user-autostop=false",
			}
			inv, root := clitest.New(t, cmdArgs...)
			clitest.SetupConfig(t, proxyClient, root)

			ctx := testutil.Context(t, testutil.WaitLong)
			err = inv.WithContext(ctx).Run()
			require.NoError(t, err)

			require.EqualValues(t, 1, atomic.LoadInt64(&updateTemplateCalled))

			// Assert that the template metadata did not change. We verify the
			// correct request gets sent to the server already.
			updated, err := client.Template(context.Background(), template.ID)
			require.NoError(t, err)
			assert.Equal(t, template.Name, updated.Name)
			assert.Equal(t, template.Description, updated.Description)
			assert.Equal(t, template.Icon, updated.Icon)
			assert.Equal(t, template.DisplayName, updated.DisplayName)
			assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
			assert.Equal(t, template.AutostopRequirement.DaysOfWeek, updated.AutostopRequirement.DaysOfWeek)
			assert.Equal(t, template.AutostopRequirement.Weeks, updated.AutostopRequirement.Weeks)
			assert.Equal(t, template.AutostartRequirement.DaysOfWeek, updated.AutostartRequirement.DaysOfWeek)
			assert.Equal(t, template.AllowUserAutostart, updated.AllowUserAutostart)
			assert.Equal(t, template.AllowUserAutostop, updated.AllowUserAutostop)
			assert.Equal(t, template.FailureTTLMillis, updated.FailureTTLMillis)
			assert.Equal(t, template.TimeTilDormantMillis, updated.TimeTilDormantMillis)
		})
	})
}
