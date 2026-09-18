import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import type { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";

type MCPServerFormDialogsProps = {
	server?: TypesGen.MCPServerConfig;
	confirmingDelete: boolean;
	setConfirmingDelete: (open: boolean) => void;
	onDeleteServer?: (serverId: string) => Promise<void>;
	isDeleting: boolean;
	confirmingRegenerateSigningSecret: boolean;
	setConfirmingRegenerateSigningSecret: (open: boolean) => void;
	onRegenerateSigningSecret?: () => void;
	unsavedChanges: ReturnType<typeof useUnsavedChangesPrompt>;
};

export const MCPServerFormDialogs: FC<MCPServerFormDialogsProps> = ({
	server,
	confirmingDelete,
	setConfirmingDelete,
	onDeleteServer,
	isDeleting,
	confirmingRegenerateSigningSecret,
	setConfirmingRegenerateSigningSecret,
	onRegenerateSigningSecret,
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
			{server && onRegenerateSigningSecret && (
				<ConfirmDialog
					type="delete"
					open={confirmingRegenerateSigningSecret}
					onClose={() => setConfirmingRegenerateSigningSecret(false)}
					title="Regenerate signing secret?"
					confirmText="Regenerate"
					description={`Regenerating the signing secret for "${server.display_name}" immediately invalidates the current secret. Requests signed with it will fail until the MCP server is updated with the new secret.`}
					onConfirm={() => {
						setConfirmingRegenerateSigningSecret(false);
						onRegenerateSigningSecret();
					}}
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
