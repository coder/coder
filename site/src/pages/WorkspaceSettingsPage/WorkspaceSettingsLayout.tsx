import { workspaceSharingSettings } from "api/queries/organizations";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

const WorkspaceSettings = createContext<Workspace | undefined>(undefined);

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
	const {
		data: workspace,
		error,
		isLoading,
		isError,
	} = useQuery(workspaceByOwnerAndName(username, workspaceName));

	const sharingSettingsQuery = useQuery({
		...workspaceSharingSettings(workspace?.organization_id ?? ""),
		enabled: !!workspace,
	});
	const sharingDisabled = sharingSettingsQuery.data?.sharing_disabled ?? false;

	if (isLoading) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle(workspaceName, "Settings")}</title>

			<Margins>
				<div className="flex flex-col md:flex-row gap-10 md:gap-20 py-12">
					{isError ? (
						<ErrorAlert error={error} />
					) : (
						workspace && (
							<WorkspaceSettings.Provider value={workspace}>
								<Sidebar
									workspace={workspace}
									username={username}
									sharingDisabled={sharingDisabled}
								/>
								<Suspense fallback={<Loader />}>
									<main className="w-full">
										<Outlet />
									</main>
								</Suspense>
							</WorkspaceSettings.Provider>
						)
					)}
				</div>
			</Margins>
		</>
	);
};
