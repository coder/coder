import { Suspense } from "react";
import { useQuery } from "react-query";
import { Outlet, useMatch, useParams } from "react-router";
import {
	workspaceByOwnerAndName,
	workspacePermissions,
} from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
import { Loader } from "#/components/Loader/Loader";
import { SettingsSidebarNavItem, Sidebar } from "#/components/Sidebar/Sidebar";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { workspaceSettingsSections } from "#/modules/workspaces/permissions";
import { pageTitle } from "#/utils/page";
import { WorkspaceSettings } from "./useWorkspaceSettings";

export const WorkspaceSettingsLayout: React.FC = () => {
	const params = useParams() as {
		workspace: string;
		username: string;
	};
	const settingsMatch = useMatch("/:username/:workspace/settings/:section?");
	const workspaceName = params.workspace;
	const username = params.username.replace("@", "");
	const workspaceQuery = useQuery(
		workspaceByOwnerAndName(username, workspaceName),
	);

	const permissionsQuery = useQuery(workspacePermissions(workspaceQuery.data));

	const error = workspaceQuery.error ?? permissionsQuery.error;
	const workspace = workspaceQuery.data;
	const permissions = permissionsQuery.data;
	if (error && (!workspace || !permissions)) {
		return <ErrorAlert error={error} />;
	}
	if (!workspace || !permissions) {
		return <Loader />;
	}

	const sections = workspaceSettingsSections(workspace, permissions);
	const sectionPath = settingsMatch
		? (settingsMatch.params.section ?? "").toLowerCase()
		: undefined;
	const canViewPage = sections.some(
		(section) => section.path === sectionPath && section.allowed,
	);

	return (
		<>
			<title>{pageTitle(workspaceName, "Workspace settings")}</title>

			<div>
				<Breadcrumb>
					<BreadcrumbList>
						<BreadcrumbItem>
							<BreadcrumbPage>Workspace settings</BreadcrumbPage>
						</BreadcrumbItem>
						<BreadcrumbSeparator />
						<BreadcrumbItem>
							<BreadcrumbPage className="flex items-center gap-2">
								<Avatar
									size="sm"
									fallback={workspace.owner_name}
									src={workspace.owner_avatar_url}
								/>
								{workspace.owner_name}
							</BreadcrumbPage>
						</BreadcrumbItem>
						<BreadcrumbSeparator />
						<BreadcrumbItem>
							<BreadcrumbLink to=".." className="flex items-center gap-2">
								<Avatar
									variant="icon"
									size="sm"
									fallback={
										workspace.template_display_name || workspace.template_name
									}
									src={workspace.template_icon}
								/>
								{workspace.name}
							</BreadcrumbLink>
						</BreadcrumbItem>
					</BreadcrumbList>
				</Breadcrumb>
				<div className="h-px border-none bg-border" />

				<section className="px-4 sm:px-6 lg:px-10 max-w-(--breakpoint-2xl) mx-auto">
					<div className="flex flex-col gap-8 py-6 lg:flex-row lg:gap-28 lg:py-10">
						<WorkspaceSettings.Provider
							value={{ owner: username, workspace, permissions }}
						>
							<Sidebar>
								<div className="flex flex-col gap-1">
									{sections
										.filter((section) => section.allowed)
										.map(({ path, label }) => (
											<SettingsSidebarNavItem
												key={path}
												end={path === ""}
												href={path}
											>
												{label}
											</SettingsSidebarNavItem>
										))}
								</div>
							</Sidebar>
							<div className="grow min-w-0">
								{Boolean(error) && <ErrorAlert error={error} />}
								<Suspense fallback={<Loader />}>
									<RequirePermission isFeatureVisible={canViewPage}>
										<Outlet />
									</RequirePermission>
								</Suspense>
							</div>
						</WorkspaceSettings.Provider>
					</div>
				</section>
			</div>
		</>
	);
};
