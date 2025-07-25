import { API } from "api/api";
import type { Workspace } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useMutation } from "react-query";

interface UseBatchActionsProps {
	onSuccess: () => Promise<void>;
}

export function useBatchActions(options: UseBatchActionsProps) {
	const { onSuccess } = options;

	const startAllMutation = useMutation({
		mutationFn: (workspaces: readonly Workspace[]) => {
			return Promise.all(
				workspaces.map((w) =>
					API.startWorkspace(w.id, w.latest_build.template_version_id),
				),
			);
		},
		onSuccess,
		onError: () => {
			displayError("Failed to start workspaces");
		},
	});

	const stopAllMutation = useMutation({
		mutationFn: (workspaces: readonly Workspace[]) => {
			return Promise.all(workspaces.map((w) => API.stopWorkspace(w.id)));
		},
		onSuccess,
		onError: () => {
			displayError("Failed to stop workspaces");
		},
	});

	const deleteAllMutation = useMutation({
		mutationFn: (workspaces: readonly Workspace[]) => {
			return Promise.all(workspaces.map((w) => API.deleteWorkspace(w.id)));
		},
		onSuccess,
		onError: () => {
			displayError("Failed to delete some workspaces");
		},
	});

	const updateAllMutation = useMutation({
		mutationFn: (payload: {
			workspaces: readonly Workspace[];
			isDynamicParametersEnabled: boolean;
		}) => {
			const { workspaces, isDynamicParametersEnabled } = payload;
			return Promise.all(
				workspaces
					.filter((w) => w.outdated && !w.dormant_at)
					.map((w) => API.updateWorkspace(w, [], isDynamicParametersEnabled)),
			);
		},
		onSuccess,
		onError: () => {
			displayError("Failed to update some workspaces");
		},
	});

	const favoriteAllMutation = useMutation({
		mutationFn: (workspaces: readonly Workspace[]) => {
			return Promise.all(
				workspaces
					.filter((w) => !w.favorite)
					.map((w) => API.putFavoriteWorkspace(w.id)),
			);
		},
		onSuccess,
		onError: () => {
			displayError("Failed to favorite some workspaces");
		},
	});

	const unfavoriteAllMutation = useMutation({
		mutationFn: (workspaces: readonly Workspace[]) => {
			return Promise.all(
				workspaces
					.filter((w) => w.favorite)
					.map((w) => API.deleteFavoriteWorkspace(w.id)),
			);
		},
		onSuccess,
		onError: () => {
			displayError("Failed to unfavorite some workspaces");
		},
	});

	return {
		favoriteAll: favoriteAllMutation.mutateAsync,
		unfavoriteAll: unfavoriteAllMutation.mutateAsync,
		startAll: startAllMutation.mutateAsync,
		stopAll: stopAllMutation.mutateAsync,
		deleteAll: deleteAllMutation.mutateAsync,
		updateAll: updateAllMutation.mutateAsync,
		isLoading:
			favoriteAllMutation.isPending ||
			unfavoriteAllMutation.isPending ||
			startAllMutation.isPending ||
			stopAllMutation.isPending ||
			deleteAllMutation.isPending,
	};
}
