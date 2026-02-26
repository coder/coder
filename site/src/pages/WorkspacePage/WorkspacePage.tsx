import { watchWorkspace } from "api/api";
import { workspaceSharingSettings } from "api/queries/organizations";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceBuildsKey } from "api/queries/workspaceBuilds";
import {
	workspaceByOwnerAndName,
	workspacePermissions,
} from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { useEffectEvent } from "hooks/hookPolyfills";
import { type FC, useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router";
import { toast } from "sonner";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";

const WorkspacePage: FC = () => {
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
	const templateQuery = useQuery({
		...templateQueryOptions(workspace?.template_id ?? ""),
		enabled: !!workspace,
	});
	const template = templateQuery.data;

	// Permissions
	const permissionsQuery = useQuery(workspacePermissions(workspace));
	const permissions = permissionsQuery.data;

	const sharingSettingsQuery = useQuery({
		...workspaceSharingSettings(workspace?.organization_id ?? ""),
		enabled: !!workspace,
	});
	const sharingDisabled = sharingSettingsQuery.data?.sharing_disabled ?? false;

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
				await queryClient.invalidateQueries({
					queryKey: workspaceBuildsKey(newWorkspaceData.id),
				});
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
			if (event.parseError) {
				toast.error(
					`Unable to process latest data for workspace "${workspaceName}".`,
					{
						description: "Please try refreshing the page.",
					},
				);
				return;
			}

			if (event.parsedMessage.type === "data") {
				updateWorkspaceData(event.parsedMessage.data as Workspace);
			}
		});
		socket.addEventListener("error", () => {
			toast.error(`Unable to get changes for workspace "${workspaceName}".`, {
				description: "Connection has been closed.",
			});
		});

		return () => socket.close();
	}, [updateWorkspaceData, workspaceId, workspaceName]);

	// Page statuses
	const pageError =
		workspaceQuery.error ?? templateQuery.error ?? permissionsQuery.error;
	const isLoading = !workspace || !template || !permissions;

	return pageError ? (
		<Margins>
			<ErrorAlert error={pageError} css={{ marginTop: 16, marginBottom: 16 }} />
		</Margins>
	) : isLoading ? (
		<Loader />
	) : (
		<WorkspaceReadyPage
			workspace={workspace}
			template={template}
			permissions={permissions}
			sharingDisabled={sharingDisabled}
		/>
	);
};

export default WorkspacePage;
