import { watchWorkspace } from "api/api";
import { checkAuthorization } from "api/queries/authCheck";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceBuildsKey } from "api/queries/workspaceBuilds";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { useEffectEvent } from "hooks/hookPolyfills";
import { AnnouncementBanners } from "modules/dashboard/AnnouncementBanners/AnnouncementBanners";
import { Navbar } from "modules/dashboard/Navbar/Navbar";
import { type FC, useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";
import { type WorkspacePermissions, workspaceChecks } from "./permissions";

export const WorkspacePage: FC = () => {
	const queryClient = useQueryClient();
	const params = useParams() as {
		username: string;
		workspace: string;
	};
	const workspaceName = params.workspace;
	const username = params.username.replace("@", "");

	// Workspace
	const workspaceQueryOptions = workspaceByOwnerAndName(
		username,
		workspaceName,
	);
	const workspaceQuery = useQuery(workspaceQueryOptions);
	const workspace = workspaceQuery.data;

	// Template
	const templateQuery = useQuery(
		workspace
			? templateQueryOptions(workspace.template_id)
			: { enabled: false },
	);
	const template = templateQuery.data;

	// Permissions
	const checks =
		workspace && template ? workspaceChecks(workspace, template) : {};
	const permissionsQuery = useQuery({
		...checkAuthorization({ checks }),
		enabled: workspace !== undefined && template !== undefined,
	});
	const permissions = permissionsQuery.data as WorkspacePermissions | undefined;

	// Watch workspace changes
	const updateWorkspaceData = useEffectEvent(
		async (newWorkspaceData: Workspace) => {
			if (!workspace) {
				throw new Error(
					"Applying an update for a workspace that is undefined.",
				);
			}

			queryClient.setQueryData(
				workspaceQueryOptions.queryKey,
				newWorkspaceData,
			);

			const hasNewBuild =
				newWorkspaceData.latest_build.id !== workspace.latest_build.id;
			const lastBuildHasChanged =
				newWorkspaceData.latest_build.status !== workspace.latest_build.status;

			if (hasNewBuild || lastBuildHasChanged) {
				await queryClient.invalidateQueries(
					workspaceBuildsKey(newWorkspaceData.id),
				);
			}
		},
	);
	const workspaceId = workspace?.id;
	useEffect(() => {
		if (!workspaceId) {
			return;
		}

		const socket = watchWorkspace(workspaceId);
		socket.addEventListener("message", (event) => {
			const newWorkspaceData = JSON.parse(event.data) as Workspace;
			void updateWorkspaceData(newWorkspaceData);
		});
		socket.addEventListener("error", (event) => {
			console.error("Error on getting workspace changes.", event);
		});

		return () => socket.close();
	}, [updateWorkspaceData, workspaceId]);

	// Page statuses
	const pageError =
		workspaceQuery.error ?? templateQuery.error ?? permissionsQuery.error;
	const isLoading = !workspace || !template || !permissions;

	return (
		<>
			<AnnouncementBanners />
			<div css={{ height: "100%", display: "flex", flexDirection: "column" }}>
				<Navbar />
				{pageError ? (
					<Margins>
						<ErrorAlert
							error={pageError}
							css={{ marginTop: 16, marginBottom: 16 }}
						/>
					</Margins>
				) : isLoading ? (
					<Loader />
				) : (
					<WorkspaceReadyPage
						workspace={workspace}
						template={template}
						permissions={permissions}
					/>
				)}
			</div>
		</>
	);
};

export default WorkspacePage;
