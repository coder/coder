import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { pageTitle } from "#/utils/page";
import { MCPServerForm } from "../components/MCPServerForm";

interface AddMCPServerPageViewProps {
	isSaving: boolean;
	externalAuthProviders: readonly TypesGen.ExternalAuthLinkProvider[];
	isLoadingExternalAuthProviders: boolean;
	externalAuthProvidersError?: unknown;
	accessURL: string;
	onCreateServer: (
		req: TypesGen.CreateMCPServerConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
}

const AddMCPServerPageView: FC<AddMCPServerPageViewProps> = ({
	isSaving,
	externalAuthProviders,
	isLoadingExternalAuthProviders,
	externalAuthProvidersError,
	accessURL,
	onCreateServer,
	onCancel,
}) => {
	return (
		<>
			<title>{pageTitle("Add server", "AI Settings")}</title>
			<MCPServerForm
				isSaving={isSaving}
				externalAuthProviders={externalAuthProviders}
				isLoadingExternalAuthProviders={isLoadingExternalAuthProviders}
				externalAuthProvidersError={externalAuthProvidersError}
				accessURL={accessURL}
				onCreateServer={onCreateServer}
				onCancel={onCancel}
			/>
		</>
	);
};

export default AddMCPServerPageView;
