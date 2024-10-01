import { TemplateRedirectController } from "pages/TemplatePage/TemplateRedirectController";
import { Suspense, lazy } from "react";
import {
	Navigate,
	Outlet,
	Route,
	createBrowserRouter,
	createRoutesFromChildren,
} from "react-router-dom";
import { Loader } from "./components/Loader/Loader";
import { RequireAuth } from "./contexts/auth/RequireAuth";
import { DashboardLayout } from "./modules/dashboard/DashboardLayout";
import { ManagementSettingsLayout } from "./modules/management/ManagementSettingsLayout";
import AuditPage from "./pages/AuditPage/AuditPage";
import { HealthLayout } from "./pages/HealthPage/HealthLayout";
import LoginPage from "./pages/LoginPage/LoginPage";
import { SetupPage } from "./pages/SetupPage/SetupPage";
import { TemplateLayout } from "./pages/TemplatePage/TemplateLayout";
import { TemplateSettingsLayout } from "./pages/TemplateSettingsPage/TemplateSettingsLayout";
import TemplatesPage from "./pages/TemplatesPage/TemplatesPage";
import UserSettingsLayout from "./pages/UserSettingsPage/Layout";
import { UsersLayout } from "./pages/UsersPage/UsersLayout";
import UsersPage from "./pages/UsersPage/UsersPage";
import { WorkspaceSettingsLayout } from "./pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import WorkspacesPage from "./pages/WorkspacesPage/WorkspacesPage";

// Lazy load pages
// - Pages that are secondary, not in the main navigation or not usually accessed
// - Pages that use heavy dependencies like charts or time libraries
const NotFoundPage = lazy(() => import("./pages/404Page/404Page"));
const CliAuthenticationPage = lazy(
	() => import("./pages/CliAuthPage/CliAuthPage"),
);
const AccountPage = lazy(
	() => import("./pages/UserSettingsPage/AccountPage/AccountPage"),
);
const AppearancePage = lazy(
	() => import("./pages/UserSettingsPage/AppearancePage/AppearancePage"),
);
const SchedulePage = lazy(
	() => import("./pages/UserSettingsPage/SchedulePage/SchedulePage"),
);
const SecurityPage = lazy(
	() => import("./pages/UserSettingsPage/SecurityPage/SecurityPage"),
);
const SSHKeysPage = lazy(
	() => import("./pages/UserSettingsPage/SSHKeysPage/SSHKeysPage"),
);
const TokensPage = lazy(
	() => import("./pages/UserSettingsPage/TokensPage/TokensPage"),
);
const WorkspaceProxyPage = lazy(
	() =>
		import("./pages/UserSettingsPage/WorkspaceProxyPage/WorkspaceProxyPage"),
);
const CreateUserPage = lazy(
	() => import("./pages/CreateUserPage/CreateUserPage"),
);
const WorkspaceBuildPage = lazy(
	() => import("./pages/WorkspaceBuildPage/WorkspaceBuildPage"),
);
const WorkspacePage = lazy(() => import("./pages/WorkspacePage/WorkspacePage"));
const WorkspaceSchedulePage = lazy(
	() =>
		import(
			"./pages/WorkspaceSettingsPage/WorkspaceSchedulePage/WorkspaceSchedulePage"
		),
);
const WorkspaceParametersPage = lazy(
	() =>
		import(
			"./pages/WorkspaceSettingsPage/WorkspaceParametersPage/WorkspaceParametersPage"
		),
);
const TerminalPage = lazy(() => import("./pages/TerminalPage/TerminalPage"));
const TemplatePermissionsPage = lazy(
	() =>
		import(
			"./pages/TemplateSettingsPage/TemplatePermissionsPage/TemplatePermissionsPage"
		),
);
const TemplateSummaryPage = lazy(
	() => import("./pages/TemplatePage/TemplateSummaryPage/TemplateSummaryPage"),
);
const CreateWorkspacePage = lazy(
	() => import("./pages/CreateWorkspacePage/CreateWorkspacePage"),
);
const CreateGroupPage = lazy(
	() => import("./pages/GroupsPage/CreateGroupPage"),
);
const GroupPage = lazy(() => import("./pages/GroupsPage/GroupPage"));
const SettingsGroupPage = lazy(
	() => import("./pages/GroupsPage/SettingsGroupPage"),
);
const GeneralSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/GeneralSettingsPage/GeneralSettingsPage"
		),
);
const SecuritySettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/SecuritySettingsPage/SecuritySettingsPage"
		),
);
const AppearanceSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/AppearanceSettingsPage/AppearanceSettingsPage"
		),
);
const UserAuthSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/UserAuthSettingsPage/UserAuthSettingsPage"
		),
);
const ExternalAuthSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/ExternalAuthSettingsPage/ExternalAuthSettingsPage"
		),
);
const OAuth2AppsSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/OAuth2AppsSettingsPage/OAuth2AppsSettingsPage"
		),
);
const EditOAuth2AppPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/OAuth2AppsSettingsPage/EditOAuth2AppPage"
		),
);
const CreateOAuth2AppPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/OAuth2AppsSettingsPage/CreateOAuth2AppPage"
		),
);
const NetworkSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/NetworkSettingsPage/NetworkSettingsPage"
		),
);
const ObservabilitySettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/ObservabilitySettingsPage/ObservabilitySettingsPage"
		),
);
const ExternalAuthPage = lazy(
	() => import("./pages/ExternalAuthPage/ExternalAuthPage"),
);
const UserExternalAuthSettingsPage = lazy(
	() => import("./pages/UserSettingsPage/ExternalAuthPage/ExternalAuthPage"),
);
const UserOAuth2ProviderSettingsPage = lazy(
	() =>
		import("./pages/UserSettingsPage/OAuth2ProviderPage/OAuth2ProviderPage"),
);
const TemplateVersionPage = lazy(
	() => import("./pages/TemplateVersionPage/TemplateVersionPage"),
);
const TemplateVersionEditorPage = lazy(
	() => import("./pages/TemplateVersionEditorPage/TemplateVersionEditorPage"),
);
const CreateTemplateGalleryPage = lazy(
	() => import("./pages/CreateTemplateGalleryPage/CreateTemplateGalleryPage"),
);
const StarterTemplatePage = lazy(
	() => import("pages/StarterTemplatePage/StarterTemplatePage"),
);
const CreateTemplatePage = lazy(
	() => import("./pages/CreateTemplatePage/CreateTemplatePage"),
);
const TemplateVariablesPage = lazy(
	() =>
		import(
			"./pages/TemplateSettingsPage/TemplateVariablesPage/TemplateVariablesPage"
		),
);
const WorkspaceSettingsPage = lazy(
	() => import("./pages/WorkspaceSettingsPage/WorkspaceSettingsPage"),
);
const CreateTokenPage = lazy(
	() => import("./pages/CreateTokenPage/CreateTokenPage"),
);
const TemplateDocsPage = lazy(
	() => import("./pages/TemplatePage/TemplateDocsPage/TemplateDocsPage"),
);
const TemplateFilesPage = lazy(
	() => import("./pages/TemplatePage/TemplateFilesPage/TemplateFilesPage"),
);
const TemplateVersionsPage = lazy(
	() =>
		import("./pages/TemplatePage/TemplateVersionsPage/TemplateVersionsPage"),
);
const TemplateSchedulePage = lazy(
	() =>
		import(
			"./pages/TemplateSettingsPage/TemplateSchedulePage/TemplateSchedulePage"
		),
);
const TemplateSettingsPage = lazy(
	() =>
		import(
			"./pages/TemplateSettingsPage/TemplateGeneralSettingsPage/TemplateSettingsPage"
		),
);
const LicensesSettingsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/LicensesSettingsPage/LicensesSettingsPage"
		),
);
const AddNewLicensePage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/LicensesSettingsPage/AddNewLicensePage"
		),
);
const CreateOrganizationPage = lazy(
	() => import("./pages/ManagementSettingsPage/CreateOrganizationPage"),
);
const OrganizationSettingsPage = lazy(
	() => import("./pages/ManagementSettingsPage/OrganizationSettingsPage"),
);
const OrganizationGroupsPage = lazy(
	() => import("./pages/ManagementSettingsPage/GroupsPage/GroupsPage"),
);
const CreateOrganizationGroupPage = lazy(
	() => import("./pages/ManagementSettingsPage/GroupsPage/CreateGroupPage"),
);
const OrganizationGroupPage = lazy(
	() => import("./pages/ManagementSettingsPage/GroupsPage/GroupPage"),
);
const OrganizationGroupSettingsPage = lazy(
	() => import("./pages/ManagementSettingsPage/GroupsPage/GroupSettingsPage"),
);
const OrganizationMembersPage = lazy(
	() => import("./pages/ManagementSettingsPage/OrganizationMembersPage"),
);
const OrganizationCustomRolesPage = lazy(
	() =>
		import("./pages/ManagementSettingsPage/CustomRolesPage/CustomRolesPage"),
);
const OrganizationIdPSyncPage = lazy(
	() => import("./pages/ManagementSettingsPage/IdpSyncPage/IdpSyncPage"),
);
const CreateEditRolePage = lazy(
	() =>
		import("./pages/ManagementSettingsPage/CustomRolesPage/CreateEditRolePage"),
);
const OrganizationProvisionersPage = lazy(
	() => import("./pages/ManagementSettingsPage/OrganizationProvisionersPage"),
);
const TemplateEmbedPage = lazy(
	() => import("./pages/TemplatePage/TemplateEmbedPage/TemplateEmbedPage"),
);
const TemplateInsightsPage = lazy(
	() =>
		import("./pages/TemplatePage/TemplateInsightsPage/TemplateInsightsPage"),
);
const GroupsPage = lazy(() => import("./pages/GroupsPage/GroupsPage"));
const IconsPage = lazy(() => import("./pages/IconsPage/IconsPage"));
const AccessURLPage = lazy(() => import("./pages/HealthPage/AccessURLPage"));
const DatabasePage = lazy(() => import("./pages/HealthPage/DatabasePage"));
const DERPPage = lazy(() => import("./pages/HealthPage/DERPPage"));
const DERPRegionPage = lazy(() => import("./pages/HealthPage/DERPRegionPage"));
const WebsocketPage = lazy(() => import("./pages/HealthPage/WebsocketPage"));
const WorkspaceProxyHealthPage = lazy(
	() => import("./pages/HealthPage/WorkspaceProxyPage"),
);
const ProvisionerDaemonsHealthPage = lazy(
	() => import("./pages/HealthPage/ProvisionerDaemonsPage"),
);
const UserNotificationsPage = lazy(
	() => import("./pages/UserSettingsPage/NotificationsPage/NotificationsPage"),
);
const DeploymentNotificationsPage = lazy(
	() =>
		import(
			"./pages/DeploymentSettingsPage/NotificationsPage/NotificationsPage"
		),
);

const RoutesWithSuspense = () => {
	return (
		<Suspense fallback={<Loader fullscreen />}>
			<Outlet />
		</Suspense>
	);
};

const templateRouter = () => {
	return (
		<Route path=":template">
			<Route element={<TemplateRedirectController />}>
				<Route element={<TemplateLayout />}>
					<Route index element={<TemplateSummaryPage />} />
					<Route path="docs" element={<TemplateDocsPage />} />
					<Route path="files" element={<TemplateFilesPage />} />
					<Route path="versions" element={<TemplateVersionsPage />} />
					<Route path="embed" element={<TemplateEmbedPage />} />
					<Route path="insights" element={<TemplateInsightsPage />} />
				</Route>

				<Route path="workspace" element={<CreateWorkspacePage />} />

				<Route path="settings" element={<TemplateSettingsLayout />}>
					<Route index element={<TemplateSettingsPage />} />
					<Route path="permissions" element={<TemplatePermissionsPage />} />
					<Route path="variables" element={<TemplateVariablesPage />} />
					<Route path="schedule" element={<TemplateSchedulePage />} />
				</Route>

				<Route path="versions">
					<Route path=":version">
						<Route index element={<TemplateVersionPage />} />
					</Route>
				</Route>
			</Route>
		</Route>
	);
};

const groupsRouter = () => {
	return (
		<Route path="groups">
			<Route index element={<OrganizationGroupsPage />} />

			<Route path="create" element={<CreateOrganizationGroupPage />} />
			<Route path=":groupName" element={<OrganizationGroupPage />} />
			<Route
				path=":groupName/settings"
				element={<OrganizationGroupSettingsPage />}
			/>
		</Route>
	);
};

export const router = createBrowserRouter(
	createRoutesFromChildren(
		<Route element={<RoutesWithSuspense />}>
			<Route path="login" element={<LoginPage />} />
			<Route path="setup" element={<SetupPage />} />

			{/* Dashboard routes */}
			<Route element={<RequireAuth />}>
				<Route element={<DashboardLayout />}>
					<Route index element={<Navigate to="/workspaces" replace />} />

					<Route
						path="/external-auth/:provider"
						element={<ExternalAuthPage />}
					/>

					<Route path="/workspaces" element={<WorkspacesPage />} />

					<Route path="/starter-templates">
						<Route index element={<CreateTemplateGalleryPage />} />
						<Route path=":exampleId" element={<StarterTemplatePage />} />
					</Route>

					<Route path="/templates">
						<Route index element={<TemplatesPage />} />
						<Route path="new" element={<CreateTemplatePage />} />
						<Route path=":organization">{templateRouter()}</Route>
						{templateRouter()}
					</Route>

					<Route path="/users">
						<Route element={<UsersLayout />}>
							<Route index element={<UsersPage />} />
						</Route>

						<Route path="create" element={<CreateUserPage />} />
					</Route>

					<Route path="/groups">
						<Route element={<UsersLayout />}>
							<Route index element={<GroupsPage />} />
						</Route>

						<Route path="create" element={<CreateGroupPage />} />
						<Route path=":groupName" element={<GroupPage />} />
						<Route path=":groupName/settings" element={<SettingsGroupPage />} />
					</Route>

					<Route path="/audit" element={<AuditPage />} />

					<Route path="/organizations" element={<ManagementSettingsLayout />}>
						<Route path="new" element={<CreateOrganizationPage />} />

						{/* General settings for the default org can omit the organization name */}
						<Route index element={<OrganizationSettingsPage />} />

						<Route path=":organization">
							<Route index element={<OrganizationSettingsPage />} />
							<Route path="members" element={<OrganizationMembersPage />} />
							{groupsRouter()}
							<Route path="roles">
								<Route index element={<OrganizationCustomRolesPage />} />
								<Route path="create" element={<CreateEditRolePage />} />
								<Route path=":roleName" element={<CreateEditRolePage />} />
							</Route>
							<Route
								path="provisioners"
								element={<OrganizationProvisionersPage />}
							/>
							<Route path="idp-sync" element={<OrganizationIdPSyncPage />} />
						</Route>
					</Route>

					<Route path="/deployment" element={<ManagementSettingsLayout />}>
						<Route path="general" element={<GeneralSettingsPage />} />
						<Route path="licenses" element={<LicensesSettingsPage />} />
						<Route path="licenses/add" element={<AddNewLicensePage />} />
						<Route path="security" element={<SecuritySettingsPage />} />
						<Route
							path="observability"
							element={<ObservabilitySettingsPage />}
						/>
						<Route path="appearance" element={<AppearanceSettingsPage />} />
						<Route path="network" element={<NetworkSettingsPage />} />
						<Route path="userauth" element={<UserAuthSettingsPage />} />
						<Route
							path="external-auth"
							element={<ExternalAuthSettingsPage />}
						/>

						<Route path="oauth2-provider">
							<Route index element={<NotFoundPage />} />
							<Route path="apps">
								<Route index element={<OAuth2AppsSettingsPage />} />
								<Route path="add" element={<CreateOAuth2AppPage />} />
								<Route path=":appId" element={<EditOAuth2AppPage />} />
							</Route>
						</Route>

						<Route path="workspace-proxies" element={<WorkspaceProxyPage />} />
						<Route path="users" element={<UsersPage />} />
						<Route path="users/create" element={<CreateUserPage />} />
						{groupsRouter()}
						<Route
							path="notifications"
							element={<DeploymentNotificationsPage />}
						/>
					</Route>

					<Route path="/settings" element={<UserSettingsLayout />}>
						<Route path="account" element={<AccountPage />} />
						<Route path="appearance" element={<AppearancePage />} />
						<Route path="schedule" element={<SchedulePage />} />
						<Route path="security" element={<SecurityPage />} />
						<Route path="ssh-keys" element={<SSHKeysPage />} />
						<Route
							path="external-auth"
							element={<UserExternalAuthSettingsPage />}
						/>
						<Route
							path="oauth2-provider"
							element={<UserOAuth2ProviderSettingsPage />}
						/>
						<Route path="tokens">
							<Route index element={<TokensPage />} />
							<Route path="new" element={<CreateTokenPage />} />
						</Route>
						<Route path="notifications" element={<UserNotificationsPage />} />
					</Route>

					{/* In order for the 404 page to work properly the routes that start with
              top level parameter must be fully qualified. */}
					<Route
						path="/:username/:workspace/builds/:buildNumber"
						element={<WorkspaceBuildPage />}
					/>
					<Route
						path="/:username/:workspace/settings"
						element={<WorkspaceSettingsLayout />}
					>
						<Route index element={<WorkspaceSettingsPage />} />
						<Route path="parameters" element={<WorkspaceParametersPage />} />
						<Route path="schedule" element={<WorkspaceSchedulePage />} />
					</Route>

					<Route path="/health" element={<HealthLayout />}>
						<Route index element={<Navigate to="access-url" replace />} />
						<Route path="access-url" element={<AccessURLPage />} />
						<Route path="database" element={<DatabasePage />} />
						<Route path="derp" element={<DERPPage />} />
						<Route path="derp/regions/:regionId" element={<DERPRegionPage />} />
						<Route path="websocket" element={<WebsocketPage />} />
						<Route
							path="workspace-proxy"
							element={<WorkspaceProxyHealthPage />}
						/>
						<Route
							path="provisioner-daemons"
							element={<ProvisionerDaemonsHealthPage />}
						/>
					</Route>
					{/* Using path="*"" means "match anything", so this route
              acts like a catch-all for URLs that we don't have explicit
              routes for. */}
					<Route path="*" element={<NotFoundPage />} />
				</Route>

				{/* Pages that don't have the dashboard layout */}
				<Route path="/:username/:workspace" element={<WorkspacePage />} />
				<Route
					path="/templates/:template/versions/:version/edit"
					element={<TemplateVersionEditorPage />}
				/>
				<Route
					path="/templates/:organization/:template/versions/:version/edit"
					element={<TemplateVersionEditorPage />}
				/>
				<Route
					path="/:username/:workspace/terminal"
					element={<TerminalPage />}
				/>
				<Route path="/cli-auth" element={<CliAuthenticationPage />} />
				<Route path="/icons" element={<IconsPage />} />
			</Route>
		</Route>,
	),
);
