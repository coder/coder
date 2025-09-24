import { API } from "api/api";
import type { Workspace, WorkspaceBuild } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useMutation } from "react-query";

interface UseBatchActionsOptions {
	onSuccess: () => Promise<void>;
}

type UpdateAllPayload = Readonly<{
	workspaces: readonly Workspace[];
	isDynamicParametersEnabled: boolean;
}>;

type UseBatchActionsResult = Readonly<{
	isProcessing: boolean;
	start: (workspaces: readonly Workspace[]) => Promise<WorkspaceBuild[]>;
	stop: (workspaces: readonly Workspace[]) => Promise<WorkspaceBuild[]>;
	delete: (workspaces: readonly Workspace[]) => Promise<WorkspaceBuild[]>;
	updateTemplateVersions: (
		payload: UpdateAllPayload,
	) => Promise<WorkspaceBuild[]>;
	favorite: (payload: readonly Workspace[]) => Promise<void>;
	unfavorite: (payload: readonly Workspace[]) => Promise<void>;
}>;

export function useBatchActions(
	options: UseBatchActionsOptions,
): UseBatchActionsResult {
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
		mutationFn: (payload: UpdateAllPayload) => {
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

	// Not a great idea to return the promises from the Promise.all calls below
	// because that then gives you a void array, which doesn't make sense with
	// TypeScript's type system. Best to await them, and then have the wrapper
	// mutation function return its own void promise

	const favoriteAllMutation = useMutation({
		mutationFn: async (workspaces: readonly Workspace[]): Promise<void> => {
			await Promise.all(
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
		mutationFn: async (workspaces: readonly Workspace[]): Promise<void> => {
			await Promise.all(
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
		favorite: favoriteAllMutation.mutateAsync,
		unfavorite: unfavoriteAllMutation.mutateAsync,
		start: startAllMutation.mutateAsync,
		stop: stopAllMutation.mutateAsync,
		delete: deleteAllMutation.mutateAsync,
		updateTemplateVersions: updateAllMutation.mutateAsync,
		isProcessing:
			favoriteAllMutation.isPending ||
			unfavoriteAllMutation.isPending ||
			startAllMutation.isPending ||
			stopAllMutation.isPending ||
			deleteAllMutation.isPending,
	};
}
