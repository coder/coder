import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";
import { OrganizationPicker } from "../components/OrganizationPicker";
import { mcpServersPath } from "../organizationParam";

interface AddMCPServerPageViewProps {
	isSaving: boolean;
	canCreate: boolean;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
}

const AddMCPServerPageView: FC<AddMCPServerPageViewProps> = ({
	isSaving,
	canCreate,
	organizations,
	organization,
	onSelectOrganization,
	onCreateServer,
	onCancel,
}) => {
	return (
		<>
			<title>{pageTitle("Add server", "AI Settings")}</title>
			<OrganizationPicker
				id="mcp-add-organization"
				className="mb-6"
				organizations={organizations}
				organization={organization}
				onChange={onSelectOrganization}
				disabled={isSaving}
			/>
			{canCreate ? (
				<MCPServerForm
					listPath={mcpServersPath(organization)}
					isSaving={isSaving}
					onCreateServer={onCreateServer}
					onCancel={onCancel}
				/>
			) : (
				<Alert severity="error" prominent>
					<AlertTitle>You cannot add servers to this organization</AlertTitle>
					<AlertDescription>
						Choose an organization where you have permission to add MCP servers.
					</AlertDescription>
				</Alert>
			)}
		</>
	);
};

export default AddMCPServerPageView;
