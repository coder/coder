import React from "react"
import { Route, Routes } from "react-router-dom"
import { AuthAndNav, RequireAuth } from "./components"
import { IndexPage } from "./pages"
import { NotFoundPage } from "./pages/404"
import { CliAuthenticationPage } from "./pages/cli-auth"
import { HealthzPage } from "./pages/healthz"
import { SignInPage } from "./pages/login"
import { PreferencesPage } from "./pages/preferences"
import { TemplatesPage } from "./pages/templates"
import { TemplatePage } from "./pages/templates/[organization]/[template]"
import { CreateWorkspacePage } from "./pages/templates/[organization]/[template]/create"
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

      <Route path="preferences">
        <Route
          index
          element={
            <AuthAndNav>
              <PreferencesPage />
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
