import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import type { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";

interface MCPServerFormDialogsProps {
	server?: TypesGen.MCPServerConfig;
	confirmingDelete: boolean;
	setConfirmingDelete: (open: boolean) => void;
	onDeleteServer?: (serverId: string) => Promise<void>;
	isDeleting: boolean;
	unsavedChanges: ReturnType<typeof useUnsavedChangesPrompt>;
}

export const MCPServerFormDialogs: FC<MCPServerFormDialogsProps> = ({
	server,
	confirmingDelete,
	setConfirmingDelete,
	onDeleteServer,
	isDeleting,
	unsavedChanges,
}) => {
	return (
		<>
			{server && onDeleteServer && (
				<ConfirmDeleteDialog
					open={confirmingDelete}
					onOpenChange={setConfirmingDelete}
					entity="MCP server"
					description={`Delete "${server.display_name}"? Agents will no longer be able to use this server.`}
					onConfirm={() => void onDeleteServer(server.id)}
					isPending={isDeleting}
				/>
			)}
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={unsavedChanges.isOpen}
				onClose={unsavedChanges.onCancel}
				onConfirm={unsavedChanges.onConfirm}
				title="Unsaved changes"
				confirmText="Confirm"
				description={
					<div className="flex items-start gap-3">
						<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0" />
						<p className="m-0">
							Your updates haven't been saved. Leave anyway?
						</p>
					</div>
				}
			/>
		</>
	);
};
