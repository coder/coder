import { API } from "api/api";
import type { Task } from "api/typesGenerated";
import type { QueryClient } from "react-query";

export const taskLogsKey = (user: string, taskId: string) => [
	"tasks",
	user,
	taskId,
	"logs",
];

export const taskLogs = (user: string, taskId: string) => ({
	queryKey: taskLogsKey(user, taskId),
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
			// TODO: #22043 - If the task's workspace has a failed start,
			// we should call restartWorkspace to clean up before starting.
			// Currently we lack the full Workspace object needed to check
			// latest_build.status and latest_build.transition.
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
