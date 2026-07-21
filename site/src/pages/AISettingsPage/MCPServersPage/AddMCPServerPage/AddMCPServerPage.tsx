import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createMCPServerConfig } from "#/api/queries/chats";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import AddMCPServerPageView from "./AddMCPServerPageView";

const AddMCPServerPage: FC = () => {
	const { organization, permissions: organizationPermissions } =
		useAIResourceOrganization();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const createMutation = useMutation(createMCPServerConfig(queryClient));

	return (
		<RequirePermission
			isFeatureVisible={Boolean(organizationPermissions?.createMCPServers)}
		>
			<AddMCPServerPageView
				isSaving={createMutation.isPending}
				onCancel={() =>
					void navigate({
						pathname: "/ai/settings/mcp-servers",
						search: location.search,
					})
				}
				onCreateServer={async (req) => {
					try {
						const server = await createMutation.mutateAsync({
							organization: organization.name,
							req,
						});
						toast.success(`MCP server "${server.display_name}" added.`);
						await navigate({
							pathname: `/ai/settings/mcp-servers/${server.id}`,
							search: location.search,
						});
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add MCP server."));
					}
				}}
			/>
		</RequirePermission>
	);
};

export default AddMCPServerPage;
