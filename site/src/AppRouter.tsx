import React from "react"
import { Navigate, Route, Routes } from "react-router-dom"
import { AuthAndNav, RequireAuth } from "./components"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404"
import { CliAuthenticationPage } from "./pages/cli-auth"
import { HealthzPage } from "./pages/healthz"
import { SignInPage } from "./pages/login"
<<<<<<< Updated upstream
import { ProjectsPage } from "./pages/projects"
import { ProjectPage } from "./pages/projects/[organization]/[project]"
import { CreateWorkspacePage } from "./pages/projects/[organization]/[project]/create"
=======
import { PreferencesAccountPage } from "./pages/preferences/account"
import { PreferencesLinkedAccountsPage } from "./pages/preferences/linked-accounts"
import { PreferencesSecurityPage } from "./pages/preferences/security"
import { PreferencesSSHKeysPage } from "./pages/preferences/ssh-keys"
import { TemplatesPage } from "./pages/templates"
import { TemplatePage } from "./pages/templates/[organization]/[template]"
import { CreateWorkspacePage } from "./pages/templates/[organization]/[template]/create"
>>>>>>> Stashed changes
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

      <Route path="projects">
        <Route
          index
          element={
            <AuthAndNav>
              <ProjectsPage />
            </AuthAndNav>
          }
        />
        <Route path=":organization/:project">
          <Route
            index
            element={
              <AuthAndNav>
                <ProjectPage />
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

<<<<<<< Updated upstream
=======
      <Route path="preferences">
        <Route index element={<Navigate to="account" />} />
        <Route
          path="account"
          element={
            <AuthAndNav>
              <PreferencesAccountPage />
            </AuthAndNav>
          }
        />
        <Route
          path="security"
          element={
            <AuthAndNav>
              <PreferencesSecurityPage />
            </AuthAndNav>
          }
        />
        <Route
          path="ssh-keys"
          element={
            <AuthAndNav>
              <PreferencesSSHKeysPage />
            </AuthAndNav>
          }
        />
        <Route
          path="linked-accounts"
          element={
            <AuthAndNav>
              <PreferencesLinkedAccountsPage />
            </AuthAndNav>
          }
        />
      </Route>

>>>>>>> Stashed changes
      {/* Using path="*"" means "match anything", so this route
        acts like a catch-all for URLs that we don't have explicit
        routes for. */}
      <Route path="*" element={<NotFoundPage />} />
    </Route>
  </Routes>
)
