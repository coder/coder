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
			return API.pauseTask(task.owner_name, task.id);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
	};
};

export const resumeTask = (task: Task, queryClient: QueryClient) => {
	return {
		mutationFn: async () => {
			return API.resumeTask(task.owner_name, task.id);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
	};
};
