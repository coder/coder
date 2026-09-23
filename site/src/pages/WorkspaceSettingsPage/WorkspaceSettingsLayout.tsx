import type { FC } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import {
	workspaceByOwnerAndName,
	workspacePermissions,
} from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { SettingsNavigation } from "#/components/SettingsNavigation/SettingsNavigation";
import { pageTitle } from "#/utils/page";
import { workspaceSettingsNavigation } from "./navigation";
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
			{error ? (
				<section className="mx-auto w-full max-w-(--breakpoint-2xl) px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
					<ErrorAlert error={error} />
				</section>
			) : (
				workspace && (
					<WorkspaceSettings.Provider
						value={{
							owner: username,
							workspace,
							permissions: permissionsQuery.data,
						}}
					>
						<SettingsNavigation
							title="Workspace settings"
							sections={workspaceSettingsNavigation(
								`/@${username}/${workspaceName}/settings`,
								permissionsQuery.data?.shareWorkspace ?? false,
							)}
							storageKey="workspace-settings-nav-collapsed"
						>
							<Outlet />
						</SettingsNavigation>
					</WorkspaceSettings.Provider>
				)
			)}
		</>
	);
};
