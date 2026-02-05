import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import type { Task } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { type QueryClient, useMutation, useQueryClient } from "react-query";

/**
 * Mutation config for pausing a task by stopping its workspace.
 */
function pauseTaskMutation(task: Task, queryClient: QueryClient) {
	return {
		mutationFn: async () => {
			if (!task.workspace_id) {
				throw new Error("Task has no workspace");
			}
			return API.stopWorkspace(task.workspace_id);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
	};
}

/**
 * Hook for pausing a task. Shows a toast on error.
 */
export function usePauseTask(task: Task) {
	const queryClient = useQueryClient();
	return useMutation({
		...pauseTaskMutation(task, queryClient),
		onError: (error: unknown) => {
			displayError(getErrorMessage(error, "Failed to pause task."));
		},
	});
}

/**
 * Mutation config for resuming a task by starting its workspace.
 */
export function resumeTaskMutation(task: Task, queryClient: QueryClient) {
	return {
		mutationFn: async () => {
			if (!task.workspace_id) {
				throw new Error("Task has no workspace");
			}
			return API.startWorkspace(
				task.workspace_id,
				task.template_version_id,
				undefined,
				undefined,
			);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
	};
}

/**
 * Hook for resuming a task. Shows a toast on error.
 */
export function useResumeTask(task: Task) {
	const queryClient = useQueryClient();
	return useMutation({
		...resumeTaskMutation(task, queryClient),
		onError: (error: unknown) => {
			displayError(getErrorMessage(error, "Failed to resume task."));
		},
	});
}
