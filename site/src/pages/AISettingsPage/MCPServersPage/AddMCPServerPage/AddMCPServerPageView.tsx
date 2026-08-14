import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";
import { OrganizationPicker } from "../components/OrganizationPicker";
import { mcpServersPath } from "../organizationParam";

interface AddMCPServerPageViewProps {
	isSaving: boolean;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
}

const AddMCPServerPageView: FC<AddMCPServerPageViewProps> = ({
	isSaving,
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
			<MCPServerForm
				listPath={mcpServersPath(organization)}
				isSaving={isSaving}
				onCreateServer={onCreateServer}
				onCancel={onCancel}
			/>
		</>
	);
};

export default AddMCPServerPageView;
