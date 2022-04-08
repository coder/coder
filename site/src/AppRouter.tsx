import React from "react"
import { Route, Routes } from "react-router-dom"
import { AuthAndNav, RequireAuth } from "./components"
import { PreferencesLayout } from "./components/Preferences/Layout"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404"
import { CliAuthenticationPage } from "./pages/cli-auth"
import { HealthzPage } from "./pages/healthz"
import { SignInPage } from "./pages/login"
import { OrganizationsPage } from "./pages/orgs"
import { PreferencesAccountPage } from "./pages/preferences/account"
import { PreferencesLinkedAccountsPage } from "./pages/preferences/linked-accounts"
import { PreferencesSecurityPage } from "./pages/preferences/security"
import { PreferencesSSHKeysPage } from "./pages/preferences/ssh-keys"
import { SettingsPage } from "./pages/settings"
import { TemplatesPage } from "./pages/templates"
import { TemplatePage } from "./pages/templates/[organization]/[template]"
import { CreateWorkspacePage } from "./pages/templates/[organization]/[template]/create"
import { UsersPage } from "./pages/users"
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
            <AuthAndNav>
              <TemplatesPage />
            </AuthAndNav>
          }
        />
        <Route path=":organization/:template">
          <Route
            index
            element={
              <AuthAndNav>
                <TemplatePage />
              </AuthAndNav>
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
            <AuthAndNav>
              <WorkspacePage />
            </AuthAndNav>
          }
        />
      </Route>

      <Route
        path="users"
        element={
          <AuthAndNav>
            <UsersPage />
          </AuthAndNav>
        }
      />
      <Route
        path="orgs"
        element={
          <AuthAndNav>
            <OrganizationsPage />
          </AuthAndNav>
        }
      />
      <Route
        path="settings"
        element={
          <AuthAndNav>
            <SettingsPage />
          </AuthAndNav>
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
