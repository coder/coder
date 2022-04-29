import React from "react"
import { Route, Routes } from "react-router-dom"
import { AuthAndFrame } from "./components/AuthAndFrame/AuthAndFrame"
import { PreferencesLayout } from "./components/PreferencesLayout/PreferencesLayout"
import { RequireAuth } from "./components/RequireAuth/RequireAuth"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404Page/404Page"
import { CliAuthenticationPage } from "./pages/CliAuthPage/CliAuthPage"
import { HealthzPage } from "./pages/HealthzPage/HealthzPage"
import { LoginPage } from "./pages/LoginPage/LoginPage"
import { OrgsPage } from "./pages/OrgsPage/OrgsPage"
import { AccountPage } from "./pages/PreferencesPages/AccountPage/AccountPage"
import { LinkedAccountsPage } from "./pages/PreferencesPages/LinkedAccountsPage/LinkedAccountsPage"
import { SecurityPage } from "./pages/PreferencesPages/SecurityPage/SecurityPage"
import { SSHKeysPage } from "./pages/PreferencesPages/SSHKeysPage/SSHKeysPage"
import { SettingsPage } from "./pages/SettingsPage/SettingsPage"
import { CreateWorkspacePage } from "./pages/TemplatesPages/OrganizationPage/TemplatePage/CreateWorkspacePage"
import { TemplatePage } from "./pages/TemplatesPages/OrganizationPage/TemplatePage/TemplatePage"
import { TemplatesPage } from "./pages/TemplatesPages/TemplatesPage"
import { TerminalPage } from "./pages/TerminalPage/TerminalPage"
import { CreateUserPage } from "./pages/UsersPage/CreateUserPage/CreateUserPage"
import { UsersPage } from "./pages/UsersPage/UsersPage"
import { WorkspacePage } from "./pages/WorkspacesPage/WorkspacesPage"

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

      <Route path="login" element={<LoginPage />} />
      <Route path="healthz" element={<HealthzPage />} />
      <Route
        path="cli-auth"
        element={
          <RequireAuth>
            <CliAuthenticationPage />
          </RequireAuth>
        }
      />

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

      <Route path="users">
        <Route
          index
          element={
            <AuthAndFrame>
              <UsersPage />
            </AuthAndFrame>
          }
        />
        <Route
          path="create"
          element={
            <RequireAuth>
              <CreateUserPage />
            </RequireAuth>
          }
        />
      </Route>
      <Route
        path="orgs"
        element={
          <AuthAndFrame>
            <OrgsPage />
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
        <Route path="account" element={<AccountPage />} />
        <Route path="security" element={<SecurityPage />} />
        <Route path="ssh-keys" element={<SSHKeysPage />} />
        <Route path="linked-accounts" element={<LinkedAccountsPage />} />
      </Route>

      <Route path=":username">
        <Route path=":workspace">
          <Route
            path="terminal"
            element={
              <RequireAuth>
                <TerminalPage />
              </RequireAuth>
            }
          />
        </Route>
      </Route>

      {/* Using path="*"" means "match anything", so this route
        acts like a catch-all for URLs that we don't have explicit
        routes for. */}
      <Route path="*" element={<NotFoundPage />} />
    </Route>
  </Routes>
)
