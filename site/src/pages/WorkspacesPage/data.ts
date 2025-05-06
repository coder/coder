import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { workspaces } from "api/queries/workspaces";
import type {
	Workspace,
	WorkspaceBuild,
	WorkspacesRequest,
	WorkspacesResponse,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useState } from "react";
import {
	type QueryKey,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";

export const useWorkspacesData = (req: WorkspacesRequest) => {
	const [shouldRefetch, setShouldRefetch] = useState(true);
	const workspacesQueryOptions = workspaces(req);
	const result = useQuery({
		...workspacesQueryOptions,
		onSuccess: () => {
			setShouldRefetch(true);
		},
		onError: () => {
			setShouldRefetch(false);
		},
		refetchInterval: shouldRefetch ? 5_000 : undefined,
	});

	return {
		...result,
		queryKey: workspacesQueryOptions.queryKey,
	};
};

export const useWorkspaceUpdate = (queryKey: QueryKey) => {
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: API.updateWorkspaceVersion,
		onMutate: async (workspace) => {
			await queryClient.cancelQueries({ queryKey });
			queryClient.setQueryData<WorkspacesResponse>(queryKey, (oldResponse) => {
				if (oldResponse) {
					return assignPendingStatus(oldResponse, workspace);
				}
			});
		},
		onSuccess: (workspaceBuild) => {
			queryClient.setQueryData<WorkspacesResponse>(queryKey, (oldResponse) => {
				if (oldResponse) {
					return assignLatestBuild(oldResponse, workspaceBuild);
				}
			});
		},
		onError: (error) => {
			const message = getErrorMessage(
				error,
				"Error updating workspace version",
			);
			displayError(message);
		},
	});
};

const assignLatestBuild = (
	oldResponse: WorkspacesResponse,
	build: WorkspaceBuild,
): WorkspacesResponse => {
	return {
		...oldResponse,
		workspaces: oldResponse.workspaces.map((workspace) => {
			if (workspace.id === build.workspace_id) {
				return {
					...workspace,
					latest_build: build,
				};
			}

			return workspace;
		}),
	};
};

const assignPendingStatus = (
	oldResponse: WorkspacesResponse,
	workspace: Workspace,
): WorkspacesResponse => {
	return {
		...oldResponse,
		workspaces: oldResponse.workspaces.map((workspaceItem) => {
			if (workspaceItem.id === workspace.id) {
				return {
					...workspace,
					latest_build: {
						...workspace.latest_build,
						status: "pending",
						job: {
							...workspace.latest_build.job,
							status: "pending",
						},
					},
				} as Workspace;
			}

			return workspaceItem;
		}),
	};
};
