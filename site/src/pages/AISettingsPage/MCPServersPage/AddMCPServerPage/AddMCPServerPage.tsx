import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createMCPServerConfig } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import AddMCPServerPageView from "./AddMCPServerPageView";

const AddMCPServerPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const createMutation = useMutation(createMCPServerConfig(queryClient));

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AddMCPServerPageView
				isSaving={createMutation.isPending}
				onCancel={() => void navigate("/ai/settings/mcp-servers")}
				onCreateServer={async (req) => {
					try {
						const server = await createMutation.mutateAsync(req);
						toast.success(`MCP server "${server.display_name}" added.`);
						await navigate(`/ai/settings/mcp-servers/${server.id}`);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add MCP server."));
					}
				}}
			/>
		</RequirePermission>
	);
};

export default AddMCPServerPage;
