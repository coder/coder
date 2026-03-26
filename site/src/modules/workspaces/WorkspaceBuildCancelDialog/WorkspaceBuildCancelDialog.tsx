import type { FC } from "react";
import type { Workspace } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";

interface WorkspaceBuildCancelDialogProps {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
	workspace: Workspace;
}

export const WorkspaceBuildCancelDialog: FC<
	WorkspaceBuildCancelDialogProps
> = ({ open, onClose, onConfirm, workspace }) => {
	const action =
		workspace.latest_build.status === "pending"
			? "remove the current build from the build queue"
			: "stop the current build process";

	return (
		<ConfirmDialog
			open={open}
			title="Cancel workspace build"
			description={`Are you sure you want to cancel the build for workspace "${workspace.name}"? This will ${action}.`}
			confirmText="Confirm"
			cancelText="Cancel"
			onClose={onClose}
			onConfirm={onConfirm}
			type="delete"
		/>
	);
};
