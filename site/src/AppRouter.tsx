import React from "react"
import { Route, Routes } from "react-router-dom"
import { AuthAndFrame } from "./components/AuthAndFrame/AuthAndFrame"
import { RequireAuth } from "./components/RequireAuth/RequireAuth"
import { SettingsLayout } from "./components/SettingsLayout/SettingsLayout"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404Page/404Page"
import { CliAuthenticationPage } from "./pages/CliAuthPage/CliAuthPage"
import { HealthzPage } from "./pages/HealthzPage/HealthzPage"
import { LoginPage } from "./pages/LoginPage/LoginPage"
import { OrgsPage } from "./pages/OrgsPage/OrgsPage"
import { SettingsPage } from "./pages/SettingsPage/SettingsPage"
import { AccountPage } from "./pages/SettingsPages/AccountPage/AccountPage"
import { SSHKeysPage } from "./pages/SettingsPages/SSHKeysPage/SSHKeysPage"
import { CreateUserPage } from "./pages/UsersPage/CreateUserPage/CreateUserPage"
import { UsersPage } from "./pages/UsersPage/UsersPage"
import { WorkspacePage } from "./pages/WorkspacePage/WorkspacePage"
import { WorkspaceSettingsPage } from "./pages/WorkspaceSettingsPage/WorkspaceSettingsPage"

const TerminalPage = React.lazy(() => import("./pages/TerminalPage/TerminalPage"))
const WorkspacesPage = React.lazy(() => import("./pages/WorkspacesPage/WorkspacesPage"))

export const AppRouter: React.FC = () => (
  <React.Suspense fallback={<></>}>
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

        <Route path="workspaces">
          <Route
            index
            element={
              <AuthAndFrame>
                <WorkspacesPage />
              </AuthAndFrame>
            }
          />
          <Route path=":workspace">
            <Route
              index
              element={
                <AuthAndFrame>
                  <WorkspacePage />
                </AuthAndFrame>
              }
            />
            <Route
              path="edit"
              element={
                <AuthAndFrame>
                  <WorkspaceSettingsPage />
                </AuthAndFrame>
              }
            />
          </Route>
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

        <Route path="settings" element={<SettingsLayout />}>
          <Route path="account" element={<AccountPage />} />
          <Route path="ssh-keys" element={<SSHKeysPage />} />
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
  </React.Suspense>
)
