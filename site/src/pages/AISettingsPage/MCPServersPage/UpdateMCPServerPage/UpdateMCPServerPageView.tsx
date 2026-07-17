import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";

interface UpdateMCPServerPageViewProps {
	server: TypesGen.MCPServerConfig;
	isSaving: boolean;
	isDeleting: boolean;
	onUpdateServer: (
		serverId: string,
		req: TypesGen.UpdateMCPServerConfigRequest,
	) => Promise<unknown>;
	onDeleteServer: (serverId: string) => Promise<void>;
	onToggleEnabled: (enabled: boolean) => void;
	onCancel: () => void;
}

const UpdateMCPServerPageView: FC<UpdateMCPServerPageViewProps> = ({
	server,
	isSaving,
	isDeleting,
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
				isSaving={isSaving}
				isDeleting={isDeleting}
				onUpdateServer={onUpdateServer}
				onDeleteServer={onDeleteServer}
				onToggleEnabled={onToggleEnabled}
				onCancel={onCancel}
			/>
		</>
	);
};

export default UpdateMCPServerPageView;
