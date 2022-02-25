import React from "react"
import CssBaseline from "@material-ui/core/CssBaseline"
import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { SWRConfig } from "swr"
import { UserProvider } from "./contexts/UserContext"
import { light } from "./theme"
import { BrowserRouter as Router, Route, Routes } from "react-router-dom"

import CliAuthenticationPage from "./pages/cli-auth"
import IndexPage from "./pages/index"
import LoginPage from "./pages/login"
import ProjectsPage from "./pages/projects"
import ProjectPage from "./pages/projects/[organization]/[project]"
import CreateWorkspacePage from "./pages/projects/[organization]/[project]/create"
import WorkspacePage from "./pages/workspaces/[user]/[workspace]"

export const App: React.FC = () => {
  return (
    <Router>
      <SWRConfig
        value={{
          fetcher: async (url: string) => {
            const res = await fetch(url)
            if (!res.ok) {
              const err = new Error((await res.json()).error?.message || res.statusText)
              throw err
            }
            return res.json()
          },
        }}
      >
        <UserProvider>
          <ThemeProvider theme={light}>
            <CssBaseline />

            <Routes>
              <Route path="/">
                <Route index element={<IndexPage />} />

                <Route path="login" element={<LoginPage />} />
                <Route path="cli-auth" element={<CliAuthenticationPage />} />

                <Route path="projects">
                  <Route index element={<ProjectsPage />} />
                  <Route path=":organization/:project">
                    <Route index element={<ProjectPage />} />
                    <Route path="create" element={<CreateWorkspacePage />} />
                  </Route>
                </Route>

                <Route path="workspaces">
                  <Route path=":user">
                    <Route path=":workspace" element={<WorkspacePage />} />
                  </Route>
                </Route>

                {/* Using path="*"" means "match anything", so this route
                acts like a catch-all for URLs that we don't have explicit
                routes for. */}
                <Route path="*" element={<div>404</div>} />
              </Route>
            </Routes>
          </ThemeProvider>
        </UserProvider>
      </SWRConfig>
    </Router>
  )
}
