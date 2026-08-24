import type { FC } from "react";
import { useQuery } from "react-query";
import { chatModelAvailability, chatModels } from "#/api/queries/chats";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { pageTitle } from "#/utils/page";
import ModelsPageView from "./ModelsPageView";
import {
	splitModelQueryErrors,
	useOrganizationModels,
} from "./organizationModels";

const ModelsPage: FC = () => {
	const { organization, permissions } = useOrganizationModels();
	const organizationModelsQuery = useQuery(chatModels(organization.id));
	const availableModelsQuery = useQuery(chatModelAvailability(organization.id));
	const providers = organizationModelsQuery.data?.providers ?? [];
	const providerTypeByID = new Map(
		providers.map((provider) => [provider.id, provider.type]),
	);
	const models = (organizationModelsQuery.data?.models ?? []).toSorted(
		(a, b) => {
			const aProvider = providerTypeByID.get(a.ai_provider_id) ?? "";
			const bProvider = providerTypeByID.get(b.ai_provider_id) ?? "";
			const cmp = aProvider.localeCompare(bProvider);
			return cmp !== 0 ? cmp : a.model.localeCompare(b.model);
		},
	);
	const providerStates = deriveProviderStates(
		models,
		providers,
		availableModelsQuery.data,
	);
	const { loadError, refetchError } = splitModelQueryErrors(
		organizationModelsQuery,
		availableModelsQuery,
	);

	return (
		<>
			<title>{pageTitle("Models", "AI Settings")}</title>

			<ModelsPageView
				key={organization.id}
				isLoading={
					organizationModelsQuery.isLoading || availableModelsQuery.isLoading
				}
				loadError={loadError}
				refetchError={refetchError}
				models={models}
				providerStates={providerStates}
				providerTypeByID={providerTypeByID}
				canCreateModel={permissions?.createChatModelConfigs ?? false}
			/>
		</>
	);
};

export default ModelsPage;
