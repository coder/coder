import {
	workspaceByOwnerAndName,
	workspacePermissions,
} from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

type WorkspaceSettingsContext = {
	owner: string;
	workspace: Workspace;
	permissions?: WorkspacePermissions;
};

const WorkspaceSettings = createContext<WorkspaceSettingsContext | undefined>(
	undefined,
);

export function useWorkspaceSettings() {
	const value = useContext(WorkspaceSettings);
	if (!value) {
		throw new Error(
			"This hook can only be used from a workspace settings page",
		);
	}

	return value;
}

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
				<Stack css={{ padding: "48px 0" }} direction="row" spacing={10}>
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
									<main css={{ width: "100%" }}>
										<Outlet />
									</main>
								</Suspense>
							</WorkspaceSettings.Provider>
						)
					)}
				</Stack>
			</Margins>
		</>
	);
};
