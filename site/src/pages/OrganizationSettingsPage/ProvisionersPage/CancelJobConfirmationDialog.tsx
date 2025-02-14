import { API } from "api/api";
import {
	getProvisionerDaemonsKey,
	provisionerJobQueryKey,
} from "api/queries/organizations";
import type { ProvisionerJob } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";

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
			queryClient.invalidateQueries(
				provisionerJobQueryKey(job.organization_id),
			);
			queryClient.invalidateQueries(
				getProvisionerDaemonsKey(job.organization_id, job.tags),
			);
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
			confirmLoading={cancelMutation.isLoading}
			onConfirm={async () => {
				try {
					await cancelMutation.mutateAsync(job);
					displaySuccess("Provisioner job canceled successfully");
					dialogProps.onClose();
				} catch {
					displayError("Failed to cancel provisioner job");
				}
			}}
		/>
	);
};
