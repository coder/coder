import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import {
	MCPServerForm,
	type MCPServerFormSaveResult,
} from "../components/MCPServerForm";
import { OrganizationPicker } from "../components/OrganizationPicker";

interface UpdateMCPServerPageViewProps {
	server: TypesGen.MCPServerConfig;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization;
	listPath: string;
	isSaving: boolean;
	isDeleting: boolean;
	isRegeneratingSigningSecret: boolean;
	canSelectUserOIDC: boolean;
	onUpdateServer?: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<MCPServerFormSaveResult | undefined>;
	onDeleteServer?: (serverId: string) => Promise<void>;
	onRegenerateSigningSecret?: () => void;
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
	isRegeneratingSigningSecret,
	canSelectUserOIDC,
	onUpdateServer,
	onDeleteServer,
	onRegenerateSigningSecret,
	onToggleEnabled,
	onCancel,
}) => {
	return (
		<>
			<title>{pageTitle(server.display_name, "AI Settings")}</title>
			<OrganizationPicker
				id="mcp-update-organization"
				className="mb-6"
				organizations={organizations}
				organization={organization}
				showSingleOrganization
			/>
			<MCPServerForm
				key={server.id}
				server={server}
				listPath={listPath}
				isSaving={isSaving}
				isDeleting={isDeleting}
				isRegeneratingSigningSecret={isRegeneratingSigningSecret}
				canSelectUserOIDC={canSelectUserOIDC}
				onUpdateServer={onUpdateServer}
				onDeleteServer={onDeleteServer}
				onRegenerateSigningSecret={onRegenerateSigningSecret}
				onToggleEnabled={onToggleEnabled}
				onCancel={onCancel}
			/>
		</>
	);
};

export default UpdateMCPServerPageView;
