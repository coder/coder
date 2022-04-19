import React from "react"
import { Route, Routes } from "react-router-dom"
import { AuthAndFrame } from "./components/AuthAndFrame/AuthAndFrame"
import { RequireAuth } from "./components/Page/RequireAuth"
import { PreferencesLayout } from "./components/Preferences/Layout"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404"
import { CliAuthenticationPage } from "./pages/cli-auth"
import { HealthzPage } from "./pages/healthz"
import { SignInPage } from "./pages/login"
import { OrganizationsPage } from "./pages/orgs"
import { PreferencesAccountPage } from "./pages/preferences/account"
import { PreferencesLinkedAccountsPage } from "./pages/preferences/linkedAccounts"
import { PreferencesSecurityPage } from "./pages/preferences/security"
import { PreferencesSSHKeysPage } from "./pages/preferences/sshKeys"
import { SettingsPage } from "./pages/settings"
import { TemplatesPage } from "./pages/templates"
import { TemplatePage } from "./pages/templates/[organization]/[template]"
import { CreateWorkspacePage } from "./pages/templates/[organization]/[template]/create"
import { UsersPage } from "./pages/UsersPage/UsersPage"
import { WorkspacePage } from "./pages/workspaces/[workspace]"

export const AppRouter: React.FC = () => (
  <Routes>
    <Route path="/">
      <Route
        index
        element={
          <RequireAuth>
            <IndexPage />
          </RequireAuth>
        }
      />

      <Route path="login" element={<SignInPage />} />
      <Route path="healthz" element={<HealthzPage />} />
      <Route path="cli-auth" element={<CliAuthenticationPage />} />

      <Route path="templates">
        <Route
          index
          element={
            <AuthAndFrame>
              <TemplatesPage />
            </AuthAndFrame>
          }
        />
        <Route path=":organization/:template">
          <Route
            index
            element={
              <AuthAndFrame>
                <TemplatePage />
              </AuthAndFrame>
            }
          />
          <Route
            path="create"
            element={
              <RequireAuth>
                <CreateWorkspacePage />
              </RequireAuth>
            }
          />
        </Route>
      </Route>

      <Route path="workspaces">
        <Route
          path=":workspace"
          element={
            <AuthAndFrame>
              <WorkspacePage />
            </AuthAndFrame>
          }
        />
      </Route>

      <Route
        path="users"
        element={
          <AuthAndFrame>
            <UsersPage />
          </AuthAndFrame>
        }
      />
      <Route
        path="orgs"
        element={
          <AuthAndFrame>
            <OrganizationsPage />
          </AuthAndFrame>
        }
      />
      <Route
        path="settings"
        element={
          <AuthAndFrame>
            <SettingsPage />
          </AuthAndFrame>
        }
      />

      <Route path="preferences" element={<PreferencesLayout />}>
        <Route path="account" element={<PreferencesAccountPage />} />
        <Route path="security" element={<PreferencesSecurityPage />} />
        <Route path="ssh-keys" element={<PreferencesSSHKeysPage />} />
        <Route path="linked-accounts" element={<PreferencesLinkedAccountsPage />} />
      </Route>

      {/* Using path="*"" means "match anything", so this route
        acts like a catch-all for URLs that we don't have explicit
        routes for. */}
      <Route path="*" element={<NotFoundPage />} />
    </Route>
  </Routes>
)
