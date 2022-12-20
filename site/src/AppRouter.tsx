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
import { AuthAndFrame } from "./components/AuthAndFrame/AuthAndFrame"
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
  () => import("./pages/DeploySettingsPage/GeneralSettingsPage"),
)
const SecuritySettingsPage = lazy(
  () => import("./pages/DeploySettingsPage/SecuritySettingsPage"),
)
const ServiceBannerSettingsPage = lazy(
  () => import("./pages/DeploySettingsPage/ServiceBannerSettingsPage"),
)
const UserAuthSettingsPage = lazy(
  () => import("./pages/DeploySettingsPage/UserAuthSettingsPage"),
)
const GitAuthSettingsPage = lazy(
  () => import("./pages/DeploySettingsPage/GitAuthSettingsPage"),
)
const NetworkSettingsPage = lazy(
  () => import("./pages/DeploySettingsPage/NetworkSettingsPage"),
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
        <Route
          index
          element={
            <RequireAuth>
              <IndexPage />
            </RequireAuth>
          }
        />

        <Route path="login" element={<LoginPage />} />
        <Route path="setup" element={<SetupPage />} />
        <Route
          path="cli-auth"
          element={
            <RequireAuth>
              <CliAuthenticationPage />
            </RequireAuth>
          }
        />
        <Route
          path="gitauth"
          element={
            <RequireAuth>
              <GitAuthPage />
            </RequireAuth>
          }
        />

        <Route
          path="workspaces"
          element={
            <AuthAndFrame>
              <WorkspacesPage />
            </AuthAndFrame>
          }
        />

        <Route path="starter-templates">
          <Route
            index
            element={
              <AuthAndFrame>
                <StarterTemplatesPage />
              </AuthAndFrame>
            }
          />

          <Route
            path=":exampleId"
            element={
              <AuthAndFrame>
                <StarterTemplatePage />
              </AuthAndFrame>
            }
          ></Route>
        </Route>

        <Route path="templates">
          <Route
            index
            element={
              <AuthAndFrame>
                <TemplatesPage />
              </AuthAndFrame>
            }
          />

          <Route
            path="new"
            element={
              <RequireAuth>
                <CreateTemplatePage />
              </RequireAuth>
            }
          />

          <Route path=":template">
            <Route
              index
              element={
                <AuthAndFrame>
                  <TemplateLayout>
                    <TemplateSummaryPage />
                  </TemplateLayout>
                </AuthAndFrame>
              }
            />
            <Route
              path="permissions"
              element={
                <AuthAndFrame>
                  <TemplateLayout>
                    <TemplatePermissionsPage />
                  </TemplateLayout>
                </AuthAndFrame>
              }
            />
            <Route
              path="workspace"
              element={
                <RequireAuth>
                  <CreateWorkspacePage />
                </RequireAuth>
              }
            />
            <Route
              path="settings"
              element={
                <RequireAuth>
                  <TemplateSettingsPage />
                </RequireAuth>
              }
            />
            <Route path="versions">
              <Route
                path=":version"
                element={
                  <AuthAndFrame>
                    <TemplateVersionPage />
                  </AuthAndFrame>
                }
              />
            </Route>
          </Route>
        </Route>

        <Route path="users">
          <Route
            index
            element={
              <AuthAndFrame>
                <UsersLayout>
                  <UsersPage />
                </UsersLayout>
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

        <Route path="/groups">
          <Route
            index
            element={
              <AuthAndFrame>
                <UsersLayout>
                  <GroupsPage />
                </UsersLayout>
              </AuthAndFrame>
            }
          />
          <Route
            path="create"
            element={
              <RequireAuth>
                <CreateGroupPage />
              </RequireAuth>
            }
          />
          <Route
            path=":groupId"
            element={
              <AuthAndFrame>
                <GroupPage />
              </AuthAndFrame>
            }
          />
          <Route
            path=":groupId/settings"
            element={
              <RequireAuth>
                <SettingsGroupPage />
              </RequireAuth>
            }
          />
        </Route>

        <Route path="/audit">
          <Route
            index
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={
                    featureVisibility[FeatureNames.AuditLog] &&
                    Boolean(permissions?.viewAuditLog)
                  }
                >
                  <AuditPage />
                </RequirePermission>
              </AuthAndFrame>
            }
          />
        </Route>

        <Route path="/settings/deployment">
          <Route
            path="general"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <GeneralSettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
          <Route
            path="security"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <SecuritySettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
          <Route
            path="service-banner"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <ServiceBannerSettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
          <Route
            path="network"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <NetworkSettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
          <Route
            path="userauth"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <UserAuthSettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
          <Route
            path="gitauth"
            element={
              <AuthAndFrame>
                <RequirePermission
                  isFeatureVisible={Boolean(permissions?.viewDeploymentConfig)}
                >
                  <DeploySettingsLayout>
                    <GitAuthSettingsPage />
                  </DeploySettingsLayout>
                </RequirePermission>
              </AuthAndFrame>
            }
          />
        </Route>

        <Route path="settings" element={<SettingsLayout />}>
          <Route path="account" element={<AccountPage />} />
          <Route path="security" element={<SecurityPage />} />
          <Route path="ssh-keys" element={<SSHKeysPage />} />
        </Route>

        <Route path="/@:username">
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
              path="schedule"
              element={
                <RequireAuth>
                  <WorkspaceSchedulePage />
                </RequireAuth>
              }
            />

            <Route
              path="terminal"
              element={
                <RequireAuth>
                  <TerminalPage />
                </RequireAuth>
              }
            />

            <Route
              path="builds/:buildNumber"
              element={
                <AuthAndFrame>
                  <WorkspaceBuildPage />
                </AuthAndFrame>
              }
            />

            <Route
              path="change-version"
              element={
                <RequireAuth>
                  <WorkspaceChangeVersionPage />
                </RequireAuth>
              }
            />
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
