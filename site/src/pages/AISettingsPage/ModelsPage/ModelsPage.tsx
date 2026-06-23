import type { FC } from "react";
import { useQuery } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import ModelsPageView from "./ModelsPageView";

const ModelsPage: FC = () => {
	const { permissions } = useAuthenticated();

	const providerConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const models = (modelConfigsQuery.data ?? []).slice().sort((a, b) => {
		const cmp = a.provider.localeCompare(b.provider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});
	const providerStates = deriveProviderStates(
		models,
		providerConfigsQuery.data,
		modelCatalogQuery.data,
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Models", "AI Settings")}</title>

			<ModelsPageView
				isLoading={
					providerConfigsQuery.isLoading ||
					modelConfigsQuery.isLoading ||
					modelCatalogQuery.isLoading
				}
				error={
					providerConfigsQuery.error ??
					modelConfigsQuery.error ??
					modelCatalogQuery.error
				}
				models={models}
				providerStates={providerStates}
			/>
		</RequirePermission>
	);
};

export default ModelsPage;
