import React from "react"
import { Routes, Route } from "react-router-dom"
import { RequireAuth, AuthAndNav } from "./components"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404"
import { CliAuthenticationPage } from "./pages/cli-auth"
import { HealthzPage } from "./pages/healthz"
import { SignInPage } from "./pages/login"
import { ProjectsPage } from "./pages/projects"
import { ProjectPage } from "./pages/projects/[organization]/[project]"
import { CreateWorkspacePage } from "./pages/projects/[organization]/[project]/create"
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

      {/* Using path="*"" means "match anything", so this route
        acts like a catch-all for URLs that we don't have explicit
        routes for. */}
      <Route path="*" element={<NotFoundPage />} />
    </Route>
  </Routes>
)
