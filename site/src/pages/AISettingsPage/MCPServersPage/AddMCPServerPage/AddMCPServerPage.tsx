import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createMCPServerConfig } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import {
	mcpServersPath,
	orgSearchParam,
	selectOrganization,
} from "../organizationParam";
import AddMCPServerPageView from "./AddMCPServerPageView";

const AddMCPServerPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const organization = selectOrganization(
		organizations,
		searchParams.get(orgSearchParam),
	);
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const createMutation = useMutation(
		createMCPServerConfig(queryClient, organization?.id ?? ""),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AddMCPServerPageView
				isSaving={createMutation.isPending}
				organizations={organizations}
				organization={organization}
				onSelectOrganization={(org) => {
					setSearchParams((params) => {
						params.set(orgSearchParam, org.name);
						return params;
					});
				}}
				onCancel={() => void navigate(mcpServersPath(organization))}
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
