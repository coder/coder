import type { FC } from "react";
import type { Workspace } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type BatchStopConfirmationProps = {
	workspacesToStop: readonly Workspace[];
	open: boolean;
	isLoading: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

export const BatchStopConfirmation: FC<BatchStopConfirmationProps> = ({
	workspacesToStop,
	open,
	onClose,
	onConfirm,
	isLoading,
}) => {
	const workspaceCount = `${workspacesToStop.length} ${
		workspacesToStop.length === 1 ? "workspace" : "workspaces"
	}`;

	return (
		<ConfirmDialog
			type="delete"
			open={open}
			onClose={onClose}
			title={`Stop ${workspaceCount}`}
			confirmLoading={isLoading}
			confirmText="Stop"
			onConfirm={onConfirm}
			description={`Are you sure you want to stop ${workspaceCount}? This will terminate all running processes and disconnect any active sessions.`}
		/>
	);
};
