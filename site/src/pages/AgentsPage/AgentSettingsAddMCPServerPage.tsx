import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createMCPServerConfig } from "#/api/queries/chats";
import { MCPServerForm } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerForm";
import { pageTitle } from "#/utils/page";

const AgentSettingsAddMCPServerPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const createMutation = useMutation(createMCPServerConfig(queryClient));

	return (
		<>
			<title>{pageTitle("Add MCP server")}</title>
			<MCPServerForm
				variant="personal"
				isSaving={createMutation.isPending}
				onCancel={() => void navigate("/agents/settings/mcp-servers")}
				onCreateServer={async (req) => {
					try {
						const server = await createMutation.mutateAsync(req);
						toast.success(`MCP server "${server.display_name}" added.`);
						await navigate(`/agents/settings/mcp-servers/${server.id}`);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add MCP server."));
					}
				}}
			/>
		</>
	);
};

export default AgentSettingsAddMCPServerPage;
