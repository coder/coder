import { API } from "api/api";
import type { Task } from "api/typesGenerated";
import type { QueryClient } from "react-query";

export const taskLogs = (user: string, taskId: string) => ({
	queryKey: ["tasks", user, taskId, "logs"],
	queryFn: () => API.getTaskLogs(user, taskId),
});

export const pauseTask = (task: Task, queryClient: QueryClient) => {
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
};

export const resumeTask = (task: Task, queryClient: QueryClient) => {
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
};
