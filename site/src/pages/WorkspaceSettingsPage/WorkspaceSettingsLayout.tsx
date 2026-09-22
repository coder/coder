import { type FC, Suspense } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
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
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import {
	DEPLOYMENT_BANNER_HEIGHT,
	useIsDeploymentBannerVisible,
} from "#/modules/dashboard/DeploymentBanner/DeploymentBanner";
import { pageTitle } from "#/utils/page";
import { WorkspaceSettings } from "./useWorkspaceSettings";
import { WorkspaceSettingsSidebar } from "./WorkspaceSettingsSidebar";
import { WorkspaceSettingsSidebarHeader } from "./WorkspaceSettingsSidebarView";

export const WorkspaceSettingsLayout: FC = () => {
	const params = useParams() as {
		workspace: string;
		username: string;
	};
	const workspaceName = params.workspace;
	const username = params.username.replace("@", "");
	const workspaceQuery = useQuery(
		workspaceByOwnerAndName(username, workspaceName),
	);
	const permissionsQuery = useQuery(workspacePermissions(workspaceQuery.data));
	const isBannerVisible = useIsDeploymentBannerVisible();

	if (workspaceQuery.isLoading) {
		return <Loader />;
	}

	const error = workspaceQuery.error || permissionsQuery.error;
	const workspace = workspaceQuery.data;

	return (
		<>
			<title>{pageTitle(workspaceName, "Workspace Settings")}</title>

			{error || !workspace ? (
				<div className="px-4 pt-6 sm:px-6 lg:px-10">
					<ErrorAlert error={error} />
				</div>
			) : (
				<WorkspaceSettings.Provider
					value={{
						owner: username,
						workspace,
						permissions: permissionsQuery.data,
					}}
				>
					<div className="flex flex-row min-h-screen">
						<div className="relative z-30 border-0 border-r border-solid border-border">
							<CollapsibleSidebar
								storageKey="workspace-settings-sidebar-width"
								header={<WorkspaceSettingsSidebarHeader />}
								bottomInset={isBannerVisible ? DEPLOYMENT_BANNER_HEIGHT : 0}
							>
								<WorkspaceSettingsSidebar />
							</CollapsibleSidebar>
						</div>
						<div className="flex-1 min-w-0">
							<Breadcrumb>
								<BreadcrumbList>
									<BreadcrumbItem>
										<BreadcrumbPage>Workspace Settings</BreadcrumbPage>
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
										<BreadcrumbLink to="..">
											<BreadcrumbPage className="flex items-center gap-2">
												<Avatar
													variant="icon"
													size="sm"
													fallback={
														workspace.template_display_name ||
														workspace.template_name
													}
													src={workspace.template_icon}
												/>
												{workspace.name}
											</BreadcrumbPage>
										</BreadcrumbLink>
									</BreadcrumbItem>
								</BreadcrumbList>
							</Breadcrumb>
							<div className="h-px border-none bg-border" />
							<div className="pt-6 pb-10 px-4 sm:px-6 lg:px-10">
								<div className="max-w-(--breakpoint-2xl) mx-auto">
									<Suspense fallback={<Loader />}>
										<Outlet />
									</Suspense>
								</div>
							</div>
						</div>
					</div>
				</WorkspaceSettings.Provider>
			)}
		</>
	);
};
