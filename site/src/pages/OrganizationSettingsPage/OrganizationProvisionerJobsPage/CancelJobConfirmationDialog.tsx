import { API } from "api/api";
import { getErrorDetail } from "api/errors";
import {
	getProvisionerDaemonsKey,
	provisionerJobsQueryKey,
} from "api/queries/organizations";
import type { ProvisionerJob } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";

type CancelJobConfirmationDialogProps = {
	open: boolean;
	onClose: () => void;
	job: ProvisionerJob;
	cancelProvisionerJob?: typeof API.cancelProvisionerJob;
};

export const CancelJobConfirmationDialog: FC<
	CancelJobConfirmationDialogProps
> = ({
	job,
	cancelProvisionerJob = API.cancelProvisionerJob,
	...dialogProps
}) => {
	const queryClient = useQueryClient();
	const cancelMutation = useMutation({
		mutationFn: cancelProvisionerJob,
		onSuccess: () => {
			queryClient.invalidateQueries({
				queryKey: provisionerJobsQueryKey(job.organization_id),
			});
			queryClient.invalidateQueries({
				queryKey: getProvisionerDaemonsKey(job.organization_id, job.tags),
			});
		},
	});

	return (
		<ConfirmDialog
			{...dialogProps}
			type="delete"
			title="Cancel provisioner job"
			description={`Are you sure you want to cancel the provisioner job "${job.id}"? This operation will result in the associated workspaces not getting created.`}
			confirmText="Confirm"
			cancelText="Discard"
			confirmLoading={cancelMutation.isPending}
			onConfirm={async () => {
				const mutation = cancelMutation.mutateAsync(job, {
					onSuccess: () => {
						dialogProps.onClose();
					},
				});
				toast.promise(mutation, {
					loading: `Canceling provisioner job "${job.id}"...`,
					success: `Provisioner job "${job.id}" canceled successfully.`,
					error: (error) => ({
						message: `Failed to cancel provisioner job "${job.id}".`,
						description: getErrorDetail(error),
					}),
				});
			}}
		/>
	);
};
