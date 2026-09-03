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
import { pageTitle } from "#/utils/page";
import { Sidebar } from "./Sidebar";
import { WorkspaceSettings } from "./useWorkspaceSettings";

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

	if (workspaceQuery.isLoading) {
		return <Loader />;
	}

	const error = workspaceQuery.error || permissionsQuery.error;
	const workspace = workspaceQuery.data;

	return (
		<>
			<title>{pageTitle(workspaceName, "Workspace Settings")}</title>

			<div>
				<Breadcrumb>
					<BreadcrumbList>
						<BreadcrumbItem>
							<BreadcrumbPage>Workspace Settings</BreadcrumbPage>
						</BreadcrumbItem>
						{workspace && (
							<>
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
							</>
						)}
					</BreadcrumbList>
				</Breadcrumb>
				<div className="h-px border-none bg-border" />

				<section className="px-4 sm:px-6 lg:px-10 max-w-(--breakpoint-2xl) mx-auto">
					<div className="flex flex-col gap-8 py-6 lg:flex-row lg:gap-28 lg:py-10">
						{error ? (
							<ErrorAlert error={error} />
						) : (
							workspaceQuery.data && (
								<WorkspaceSettings.Provider
									value={{
										owner: username,
										workspace: workspaceQuery.data,
										permissions: permissionsQuery.data,
									}}
								>
									<Sidebar />
									<div className="grow min-w-0">
										<Suspense fallback={<Loader />}>
											<Outlet />
										</Suspense>
									</div>
								</WorkspaceSettings.Provider>
							)
						)}
					</div>
				</section>
			</div>
		</>
	);
};
