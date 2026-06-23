import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";

interface AddMCPServerPageViewProps {
	isSaving: boolean;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
}

const AddMCPServerPageView: FC<AddMCPServerPageViewProps> = ({
	isSaving,
	onCreateServer,
	onCancel,
}) => {
	return (
		<>
			<title>{pageTitle("Add server", "AI Settings")}</title>
			<MCPServerForm
				isSaving={isSaving}
				isDeleting={false}
				onCreateServer={onCreateServer}
				onUpdateServer={async () => {}}
				onCancel={onCancel}
			/>
		</>
	);
};

export default AddMCPServerPageView;
