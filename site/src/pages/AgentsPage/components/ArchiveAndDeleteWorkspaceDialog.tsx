import type { Workspace } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";

type ArchiveAndDeleteWorkspaceDialogProps = {
	/** The workspace to confirm. The dialog is open while one is set. */
	readonly workspace: Workspace | undefined;
	readonly onConfirm: (workspace: Workspace) => void;
	readonly onCancel: () => void;
};

export const ArchiveAndDeleteWorkspaceDialog: React.FC<
	ArchiveAndDeleteWorkspaceDialogProps
> = ({ workspace, onConfirm, onCancel }) => (
	<DeleteDialog
		isOpen={workspace !== undefined}
		onConfirm={() => {
			if (workspace) {
				onConfirm(workspace);
			}
		}}
		onCancel={onCancel}
		entity="workspace"
		name={workspace?.name ?? ""}
		title="Archive agent & delete workspace"
		verb="Archiving and deleting"
		info="This will archive the agent and permanently delete the associated workspace and all its resources."
	/>
);
