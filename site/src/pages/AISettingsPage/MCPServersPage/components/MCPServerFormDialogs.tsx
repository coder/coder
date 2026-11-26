import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { UnsavedChangesDialog } from "#/components/Dialog/UnsavedChangesDialog/UnsavedChangesDialog";
import type { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";

type MCPServerFormDialogsProps = {
	server?: TypesGen.MCPServerConfig;
	confirmingDelete: boolean;
	setConfirmingDelete: (open: boolean) => void;
	onDeleteServer?: (serverId: string) => Promise<void>;
	isDeleting: boolean;
	unsavedChanges: ReturnType<typeof useUnsavedChangesPrompt>;
};

export const MCPServerFormDialogs: React.FC<MCPServerFormDialogsProps> = ({
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
				<ConfirmDialog
					type="delete"
					open={confirmingDelete}
					onClose={() => setConfirmingDelete(false)}
					title="Delete MCP server"
					confirmText="Delete MCP server"
					description={`Delete "${server.display_name}"? Agents will no longer be able to use this server.`}
					onConfirm={() => void onDeleteServer(server.id)}
					confirmLoading={isDeleting}
				/>
			)}
			<UnsavedChangesDialog
				open={unsavedChanges.isOpen}
				onClose={unsavedChanges.onCancel}
				onConfirm={unsavedChanges.onConfirm}
			/>
		</>
	);
};
