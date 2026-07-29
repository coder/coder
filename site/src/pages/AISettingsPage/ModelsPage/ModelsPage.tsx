import type { FC } from "react";
import { useQuery } from "react-query";
import {
	chatAIProviderCatalog,
	chatModelConfigsByOrganization,
	chatModels,
} from "#/api/queries/chats";
import {
	deriveProviderStates,
	providerConfigFromCatalogEntry,
} from "#/modules/aiModels/providerStates";
import { providerTypeByIDFromConfigs } from "#/pages/AgentsPage/utils/modelOptions";
import { pageTitle } from "#/utils/page";
import ModelsPageView from "./ModelsPageView";
import { useOrganizationModels } from "./organizationModels";

const ModelsPage: FC = () => {
	const { organization } = useOrganizationModels();

	const providerCatalogQuery = useQuery(chatAIProviderCatalog());
	const modelConfigsQuery = useQuery(
		chatModelConfigsByOrganization(organization.id),
	);
	const modelCatalogQuery = useQuery(chatModels());

	const providerConfigs = providerCatalogQuery.data?.map(
		providerConfigFromCatalogEntry,
	);
	const providerTypeByID = providerTypeByIDFromConfigs(providerConfigs);

	const models = (modelConfigsQuery.data ?? []).slice().sort((a, b) => {
		const aProvider = providerTypeByID.get(a.ai_provider_id) ?? "";
		const bProvider = providerTypeByID.get(b.ai_provider_id) ?? "";
		const cmp = aProvider.localeCompare(bProvider);
		return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
	});
	const providerStates = deriveProviderStates(
		models,
		providerConfigs,
		modelCatalogQuery.data,
	);

	return (
		<>
			<title>{pageTitle("Models", "AI Settings")}</title>

			<ModelsPageView
				isLoading={
					providerCatalogQuery.isLoading ||
					modelConfigsQuery.isLoading ||
					modelCatalogQuery.isLoading
				}
				error={
					providerCatalogQuery.error ??
					modelConfigsQuery.error ??
					modelCatalogQuery.error
				}
				models={models}
				providerStates={providerStates}
				providerTypeByID={providerTypeByID}
			/>
		</>
	);
};

export default ModelsPage;
