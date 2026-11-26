import type { Workspace } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type WorkspaceBuildCancelDialogProps = {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
	workspace: Workspace;
	onCloseAutoFocus?: (event: Event) => void;
};

export const WorkspaceBuildCancelDialog: React.FC<
	WorkspaceBuildCancelDialogProps
> = ({ open, onClose, onConfirm, workspace, onCloseAutoFocus }) => {
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
			onCloseAutoFocus={onCloseAutoFocus}
			onConfirm={onConfirm}
			type="delete"
		/>
	);
};
