import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";
import { OrganizationPicker } from "../components/OrganizationPicker";

interface UpdateMCPServerPageViewProps {
	server: TypesGen.MCPServerConfig;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization;
	listPath: string;
	isSaving: boolean;
	isDeleting: boolean;
	canSelectUserOIDC: boolean;
	canShareServer?: boolean;
	onUpdateServer?: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<unknown>;
	onDeleteServer?: (serverId: string) => Promise<void>;
	onToggleEnabled?: (enabled: boolean) => void;
	onCancel: () => void;
}

const UpdateMCPServerPageView: FC<UpdateMCPServerPageViewProps> = ({
	server,
	organizations,
	organization,
	listPath,
	isSaving,
	isDeleting,
	canSelectUserOIDC,
	canShareServer,
	onUpdateServer,
	onDeleteServer,
	onToggleEnabled,
	onCancel,
}) => {
	return (
		<>
			<title>{pageTitle(server.display_name, "AI Settings")}</title>
			<MCPServerForm
				key={server.id}
				server={server}
				listPath={listPath}
				isSaving={isSaving}
				isDeleting={isDeleting}
				canSelectUserOIDC={canSelectUserOIDC}
				canShareServer={canShareServer}
				organizationPicker={
					<OrganizationPicker
						id="mcp-update-organization"
						className="w-full"
						organizations={organizations}
						organization={organization}
						showSingleOrganization
					/>
				}
				onUpdateServer={onUpdateServer}
				onDeleteServer={onDeleteServer}
				onToggleEnabled={onToggleEnabled}
				onCancel={onCancel}
			/>
		</>
	);
};

export default UpdateMCPServerPageView;
