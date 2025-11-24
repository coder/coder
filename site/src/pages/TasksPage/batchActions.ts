import { API } from "api/api";
import type { Task } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useMutation } from "react-query";

interface UseBatchTaskActionsOptions {
	onSuccess: () => Promise<void>;
}

type UseBatchTaskActionsResult = Readonly<{
	isProcessing: boolean;
	delete: (tasks: readonly Task[]) => Promise<void>;
}>;

export function useBatchTaskActions(
	options: UseBatchTaskActionsOptions,
): UseBatchTaskActionsResult {
	const { onSuccess } = options;

	const deleteAllMutation = useMutation({
		mutationFn: async (tasks: readonly Task[]): Promise<void> => {
			await Promise.all(
				tasks.map((task) => API.deleteTask(task.owner_name, task.id)),
			);
		},
		onSuccess,
		onError: () => {
			displayError("Failed to delete some tasks");
		},
	});

	return {
		delete: deleteAllMutation.mutateAsync,
		isProcessing: deleteAllMutation.isPending,
	};
}
