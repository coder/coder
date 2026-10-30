import { useMutation, useQueryClient } from "react-query";
import {
	deleteMCPServerUserToken,
	updateMCPServerUserToken,
} from "#/api/queries/chats";
import type { MCPServerConfig } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MCPTokenDialog } from "#/modules/mcp/MCPTokenDialog";
import {
	builtInTokenHelp,
	detectBuiltInMCPServer,
} from "#/modules/mcp/mcpPresets";

type MCPUserTokenDialogProps = {
	readonly server: MCPServerConfig;
	readonly onClose: () => void;
};

/** Adds, replaces, or removes the signed-in user's token for an MCP server. */
export const MCPUserTokenDialog: React.FC<MCPUserTokenDialogProps> = ({
	server,
	onClose,
}) => {
	const queryClient = useQueryClient();
	const saveMutation = useMutation(updateMCPServerUserToken(queryClient));
	const removeMutation = useMutation(deleteMCPServerUserToken(queryClient));
	const builtIn = detectBuiltInMCPServer(server);

	return (
		<MCPTokenDialog
			serverName={server.display_name}
			audience="Your chats"
			hasCredential={server.auth_connected}
			help={builtIn ? builtInTokenHelp[builtIn] : undefined}
			isSaving={saveMutation.isPending}
			isRemoving={removeMutation.isPending}
			error={saveMutation.error ?? removeMutation.error}
			onSave={(token) =>
				saveMutation.mutate(
					{ id: server.id, token },
					{
						onSuccess: () => {
							toast.success(`Saved your token for ${server.display_name}.`);
							onClose();
						},
					},
				)
			}
			onRemove={() =>
				removeMutation.mutate(server.id, {
					onSuccess: () => {
						toast.success(`Removed your token for ${server.display_name}.`);
						onClose();
					},
				})
			}
			onClose={onClose}
		/>
	);
};
