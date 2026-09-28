import type { FC } from "react";
import { useQuery } from "react-query";
import { userChatProviderConfigs } from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsAPIKeysPageView } from "./AgentSettingsAPIKeysPageView";
import { useOrganizationChatModels } from "./hooks/useOrganizationChatModels";

const AgentSettingsAPIKeysPage: FC = () => {
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	const providersQuery = useQuery(userChatProviderConfigs());

	return (
		<AgentSettingsAPIKeysPageView
			error={providersQuery.error}
			isLoading={providersQuery.isLoading}
			providers={providersQuery.data ?? []}
			models={organizationModels.models}
			isModelsLoading={organizationModels.isLoading}
			areModelsUnavailable={Boolean(
				organizationModels.error ?? organizationModels.partialError,
			)}
		/>
	);
};

export default AgentSettingsAPIKeysPage;
