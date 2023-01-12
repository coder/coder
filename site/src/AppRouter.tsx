import { useSelector } from "@xstate/react"
import { FeatureNames } from "api/types"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { RequirePermission } from "components/RequirePermission/RequirePermission"
import { TemplateLayout } from "components/TemplateLayout/TemplateLayout"
import { UsersLayout } from "components/UsersLayout/UsersLayout"
import IndexPage from "pages"
import AuditPage from "pages/AuditPage/AuditPage"
import GroupsPage from "pages/GroupsPage/GroupsPage"
import LoginPage from "pages/LoginPage/LoginPage"
import { SetupPage } from "pages/SetupPage/SetupPage"
import { TemplateSettingsPage } from "pages/TemplateSettingsPage/TemplateSettingsPage"
import TemplatesPage from "pages/TemplatesPage/TemplatesPage"
import UsersPage from "pages/UsersPage/UsersPage"
import WorkspacesPage from "pages/WorkspacesPage/WorkspacesPage"
import { FC, lazy, Suspense, useContext } from "react"
import { Route, Routes } from "react-router-dom"
import { selectPermissions } from "xServices/auth/authSelectors"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "xServices/StateContext"
import { NavbarLayout } from "./components/NavbarLayout/NavbarLayout"
import { RequireAuth } from "./components/RequireAuth/RequireAuth"
import { SettingsLayout } from "./components/SettingsLayout/SettingsLayout"
import { DeploySettingsLayout } from "components/DeploySettingsLayout/DeploySettingsLayout"

// Lazy load pages
// - Pages that are secondary, not in the main navigation or not usually accessed
// - Pages that use heavy dependencies like charts or time libraries
const NotFoundPage = lazy(() => import("./pages/404Page/404Page"))
const CliAuthenticationPage = lazy(
  () => import("./pages/CliAuthPage/CliAuthPage"),
)
const AccountPage = lazy(
  () => import("./pages/UserSettingsPage/AccountPage/AccountPage"),
)
const SecurityPage = lazy(
  () => import("./pages/UserSettingsPage/SecurityPage/SecurityPage"),
)
const SSHKeysPage = lazy(
  () => import("./pages/UserSettingsPage/SSHKeysPage/SSHKeysPage"),
)
const CreateUserPage = lazy(
  () => import("./pages/UsersPage/CreateUserPage/CreateUserPage"),
)
const WorkspaceBuildPage = lazy(
  () => import("./pages/WorkspaceBuildPage/WorkspaceBuildPage"),
)
const WorkspacePage = lazy(() => import("./pages/WorkspacePage/WorkspacePage"))
const WorkspaceChangeVersionPage = lazy(
  () => import("./pages/WorkspaceChangeVersionPage/WorkspaceChangeVersionPage"),
)
const WorkspaceSchedulePage = lazy(
  () => import("./pages/WorkspaceSchedulePage/WorkspaceSchedulePage"),
)
const TerminalPage = lazy(() => import("./pages/TerminalPage/TerminalPage"))
const TemplatePermissionsPage = lazy(
  () =>
    import(
      "./pages/TemplatePage/TemplatePermissionsPage/TemplatePermissionsPage"
    ),
)
const TemplateSummaryPage = lazy(
  () => import("./pages/TemplatePage/TemplateSummaryPage/TemplateSummaryPage"),
)
const CreateWorkspacePage = lazy(
  () => import("./pages/CreateWorkspacePage/CreateWorkspacePage"),
)
const CreateGroupPage = lazy(() => import("./pages/GroupsPage/CreateGroupPage"))
const GroupPage = lazy(() => import("./pages/GroupsPage/GroupPage"))
const SettingsGroupPage = lazy(
  () => import("./pages/GroupsPage/SettingsGroupPage"),
)
const GeneralSettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/GeneralSettingsPage/GeneralSettingsPage"
    ),
)
const SecuritySettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/SecuritySettingsPage/SecuritySettingsPage"
    ),
)
const AppearanceSettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/AppearanceSettingsPage/AppearanceSettingsPage"
    ),
)
const UserAuthSettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/UserAuthSettingsPage/UserAuthSettingsPage"
    ),
)
const GitAuthSettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/GitAuthSettingsPage/GitAuthSettingsPage"
    ),
)
const NetworkSettingsPage = lazy(
  () =>
    import(
      "./pages/DeploySettingsPage/NetworkSettingsPage/NetworkSettingsPage"
    ),
)
const GitAuthPage = lazy(() => import("./pages/GitAuthPage/GitAuthPage"))
const TemplateVersionPage = lazy(
  () => import("./pages/TemplateVersionPage/TemplateVersionPage"),
)
const StarterTemplatesPage = lazy(
  () => import("./pages/StarterTemplatesPage/StarterTemplatesPage"),
)
const StarterTemplatePage = lazy(
  () => import("pages/StarterTemplatePage/StarterTemplatePage"),
)
const CreateTemplatePage = lazy(
  () => import("./pages/CreateTemplatePage/CreateTemplatePage"),
)

export const AppRouter: FC = () => {
  const xServices = useContext(XServiceContext)
  const permissions = useSelector(xServices.authXService, selectPermissions)
  const featureVisibility = useSelector(
    xServices.entitlementsXService,
    selectFeatureVisibility,
  )

  return (
    <Suspense fallback={<FullScreenLoader />}>
      <Routes>
        <Route path="login" element={<LoginPage />} />
        <Route path="setup" element={<SetupPage />} />

        {/* Authenticated routes */}
        <Route element={<RequireAuth />}>
          <Route index element={<IndexPage />} />

          <Route path="cli-auth" element={<CliAuthenticationPage />} />
          <Route path="gitauth" element={<GitAuthPage />} />

          <Route
            path="workspaces"
            element={
              <NavbarLayout>
                <WorkspacesPage />
              </NavbarLayout>
            }
          />

          <Route path="starter-templates">
            <Route
              index
              element={
                <NavbarLayout>
                  <StarterTemplatesPage />
                </NavbarLayout>
              }
            />

            <Route
              path=":exampleId"
              element={
                <NavbarLayout>
                  <StarterTemplatePage />
                </NavbarLayout>
              }
            />
          </Route>

          <Route path="templates">
            <Route
              index
              element={
                <NavbarLayout>
                  <TemplatesPage />
                </NavbarLayout>
              }
            />

            <Route path="new" element={<CreateTemplatePage />} />

            <Route path=":template">
              <Route
                index
                element={
                  <NavbarLayout>
                    <TemplateLayout>
                      <TemplateSummaryPage />
                    </TemplateLayout>
                  </NavbarLayout>
                }
              />
              <Route
                path="permissions"
                element={
                  <NavbarLayout>
                    <TemplateLayout>
                      <TemplatePermissionsPage />
                    </TemplateLayout>
                  </NavbarLayout>
                }
              />
              <Route path="workspace" element={<CreateWorkspacePage />} />
              <Route path="settings" element={<TemplateSettingsPage />} />
              <Route path="versions">
                <Route
                  path=":version"
                  element={
                    <NavbarLayout>
                      <TemplateVersionPage />
                    </NavbarLayout>
                  }
                />
              </Route>
            </Route>
          </Route>

          <Route path="users">
            <Route
              index
              element={
                <NavbarLayout>
                  <UsersLayout>
                    <UsersPage />
                  </UsersLayout>
                </NavbarLayout>
              }
            />
            <Route path="create" element={<CreateUserPage />} />
          </Route>

          <Route path="/groups">
            <Route
              index
              element={
                <NavbarLayout>
                  <UsersLayout>
                    <GroupsPage />
                  </UsersLayout>
                </NavbarLayout>
              }
            />
            <Route path="create" element={<CreateGroupPage />} />
            <Route
              path=":groupId"
              element={
                <NavbarLayout>
                  <GroupPage />
                </NavbarLayout>
              }
            />
            <Route path=":groupId/settings" element={<SettingsGroupPage />} />
          </Route>

          <Route path="/audit">
            <Route
              index
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={
                      featureVisibility[FeatureNames.AuditLog] &&
                      Boolean(permissions?.viewAuditLog)
                    }
                  >
                    <AuditPage />
                  </RequirePermission>
                </NavbarLayout>
              }
            />
          </Route>

          <Route path="/settings/deployment">
            <Route
              path="general"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <GeneralSettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
            <Route
              path="security"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <SecuritySettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
            <Route
              path="appearance"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <AppearanceSettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
            <Route
              path="network"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <NetworkSettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
            <Route
              path="userauth"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <UserAuthSettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
            <Route
              path="gitauth"
              element={
                <NavbarLayout>
                  <RequirePermission
                    isFeatureVisible={Boolean(
                      permissions?.viewDeploymentConfig,
                    )}
                  >
                    <DeploySettingsLayout>
                      <GitAuthSettingsPage />
                    </DeploySettingsLayout>
                  </RequirePermission>
                </NavbarLayout>
              }
            />
          </Route>

          <Route path="settings">
            <Route
              path="account"
              element={
                <NavbarLayout>
                  <SettingsLayout>
                    <AccountPage />
                  </SettingsLayout>
                </NavbarLayout>
              }
            />
            <Route
              path="security"
              element={
                <NavbarLayout>
                  <SettingsLayout>
                    <SecurityPage />
                  </SettingsLayout>
                </NavbarLayout>
              }
            />
            <Route
              path="ssh-keys"
              element={
                <NavbarLayout>
                  <SettingsLayout>
                    <SSHKeysPage />
                  </SettingsLayout>
                </NavbarLayout>
              }
            />
          </Route>

          <Route path="/@:username">
            <Route path=":workspace">
              <Route
                index
                element={
                  <NavbarLayout>
                    <WorkspacePage />
                  </NavbarLayout>
                }
              />

              <Route path="schedule" element={<WorkspaceSchedulePage />} />

              <Route path="terminal" element={<TerminalPage />} />

              <Route
                path="builds/:buildNumber"
                element={
                  <NavbarLayout>
                    <WorkspaceBuildPage />
                  </NavbarLayout>
                }
              />

              <Route
                path="change-version"
                element={<WorkspaceChangeVersionPage />}
              />
            </Route>
          </Route>
        </Route>

        {/* Using path="*"" means "match anything", so this route
        acts like a catch-all for URLs that we don't have explicit
        routes for. */}
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  )
}
