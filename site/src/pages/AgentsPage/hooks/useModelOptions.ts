import { useQuery } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements/ModelSelector";
import {
	getModelOptionsFromConfigs,
	hasConfiguredModelsInCatalog,
	providerTypeByIDFromUserConfigs,
} from "../utils/modelOptions";

interface UseModelOptionsResult {
	options: readonly ModelSelectorOption[];
	isModelCatalogLoading: boolean;
	modelCatalog: TypesGen.ChatModelsResponse | undefined;
	hasConfiguredModels: boolean;
}

/**
 * useModelOptions owns the three queries the user-facing model selector needs
 * (model configs, the model catalog, and the user provider configs) and
 * derives its own loading flag.
 *
 * Provider identity lives in a separate query (userChatProviderConfigs), so a
 * page that renders with configs loaded but that query still pending would
 * build an empty provider map, drop every option, and flash "No Models". By
 * folding all three loading states into a single derived source, that race is
 * structurally impossible for every consumer instead of being guarded per page.
 */
export const useModelOptions = (): UseModelOptionsResult => {
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const catalogQuery = useQuery(chatModels());
	const providerConfigsQuery = useQuery(userChatProviderConfigs());

	const options = getModelOptionsFromConfigs(
		modelConfigsQuery.data,
		catalogQuery.data,
		providerTypeByIDFromUserConfigs(providerConfigsQuery.data),
	);

	return {
		options,
		isModelCatalogLoading:
			modelConfigsQuery.isLoading ||
			catalogQuery.isLoading ||
			providerConfigsQuery.isLoading,
		modelCatalog: catalogQuery.data,
		hasConfiguredModels: hasConfiguredModelsInCatalog(catalogQuery.data),
	};
};
