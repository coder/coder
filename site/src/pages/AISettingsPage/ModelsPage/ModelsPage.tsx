import type { FC } from "react";
import { useQuery } from "react-query";
import { chatProviderConfigs } from "#/api/queries/aiProviders";
import { chatModelAvailability, chatModels } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { providerTypeByIDFromConfigs } from "#/pages/AgentsPage/utils/modelOptions";
import { pageTitle } from "#/utils/page";
import ModelsPageView from "./ModelsPageView";

const ModelsPage: FC = () => {
	const { permissions } = useAuthenticated();

	const providerConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const modelsQuery = useQuery(chatModels());
	const modelCatalogQuery = useQuery(chatModelAvailability());

	const providerTypeByID = providerTypeByIDFromConfigs(
		providerConfigsQuery.data,
	);

	const models = (modelsQuery.data ?? []).slice().sort((a, b) => {
		const aProvider = providerTypeByID.get(a.ai_provider_id) ?? "";
		const bProvider = providerTypeByID.get(b.ai_provider_id) ?? "";
		const cmp = aProvider.localeCompare(bProvider);
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
					modelsQuery.isLoading ||
					modelCatalogQuery.isLoading
				}
				error={
					providerConfigsQuery.error ??
					modelsQuery.error ??
					modelCatalogQuery.error
				}
				models={models}
				providerStates={providerStates}
				providerTypeByID={providerTypeByID}
			/>
		</RequirePermission>
	);
};

export default ModelsPage;
