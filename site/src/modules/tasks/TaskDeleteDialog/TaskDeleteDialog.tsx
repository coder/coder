import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type { Task } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { QueryClient, useMutation } from "react-query";

type TaskDeleteDialogProps = {
	open: boolean;
	task: Task;
	onClose: () => void;
	onSuccess?: () => void;
};

export const TaskDeleteDialog: FC<TaskDeleteDialogProps> = ({
	task,
	onSuccess,
	...props
}) => {
	const queryClient = new QueryClient();
	const deleteTaskMutation = useMutation({
		mutationFn: () => API.tasks.deleteTask(task.owner_name, task.id),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
	});

	return (
		<ConfirmDialog
			{...props}
			type="delete"
			confirmLoading={deleteTaskMutation.isPending}
			title="Delete task"
			onConfirm={async () => {
				try {
					await deleteTaskMutation.mutateAsync();
					displaySuccess("Task deleted successfully");
					onSuccess?.();
				} catch (error) {
					displayError(
						getErrorMessage(error, "Failed to delete task"),
						getErrorDetail(error),
					);
				} finally {
					props.onClose();
				}
			}}
			description={
				<p>
					This action is irreversible and removes all workspace resources and
					data.
				</p>
			}
		/>
	);
};
