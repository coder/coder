import { type FC, Suspense } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import {
	workspaceByOwnerAndName,
	workspacePermissions,
} from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { Stack } from "#/components/Stack/Stack";
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

	return (
		<>
			<title>{pageTitle(workspaceName, "Settings")}</title>

			<Margins>
				<Stack className="py-12" direction="row" spacing={10}>
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
								<Suspense fallback={<Loader />}>
									<div className="w-full">
										<Outlet />
									</div>
								</Suspense>
							</WorkspaceSettings.Provider>
						)
					)}
				</Stack>
			</Margins>
		</>
	);
};
