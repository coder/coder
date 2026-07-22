import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	updateUserChatPersonalModelOverride,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const AgentSettingsUserAgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const overridesQuery = useQuery(userChatPersonalModelOverrides());
	const chatModelsQuery = useQuery(chatModels());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const providerConfigsQuery = useQuery(userChatProviderConfigs());
	const saveRootModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient),
	);
	const saveGeneralModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient),
	);
	const { options: modelOptions, isModelCatalogLoading } = resolveModelSelector(
		modelConfigsQuery,
		chatModelsQuery,
		providerConfigsQuery,
	);
	const modelConfigsError = modelConfigsQuery.error ?? chatModelsQuery.error;

	const saveModelOverride = (
		context: TypesGen.ChatPersonalModelOverrideContext,
		mutation: typeof saveRootModelOverrideMutation,
	) => {
		return (
			req: TypesGen.UpdateUserChatPersonalModelOverrideRequest,
			options?: { onSuccess?: () => void; onError?: () => void },
		) => {
			mutation.mutate({ context, req }, options);
		};
	};

	return (
		<AgentSettingsUserAgentsPageView
			overridesData={overridesQuery.data}
			overridesError={overridesQuery.error}
			onRetryOverrides={() => {
				void overridesQuery.refetch();
			}}
			isRetryingOverrides={overridesQuery.isFetching}
			isLoadingOverrides={overridesQuery.isLoading}
			modelOptions={modelOptions}
			modelConfigs={modelConfigsQuery.data ?? []}
			modelConfigsError={modelConfigsError}
			isLoadingModels={isModelCatalogLoading}
			onSaveRootModelOverride={saveModelOverride(
				"root",
				saveRootModelOverrideMutation,
			)}
			isSavingRootModelOverride={saveRootModelOverrideMutation.isPending}
			isSaveRootModelOverrideError={saveRootModelOverrideMutation.isError}
			onSaveGeneralModelOverride={saveModelOverride(
				"general",
				saveGeneralModelOverrideMutation,
			)}
			isSavingGeneralModelOverride={saveGeneralModelOverrideMutation.isPending}
			isSaveGeneralModelOverrideError={saveGeneralModelOverrideMutation.isError}
			onSaveExploreModelOverride={saveModelOverride(
				"explore",
				saveExploreModelOverrideMutation,
			)}
			isSavingExploreModelOverride={saveExploreModelOverrideMutation.isPending}
			isSaveExploreModelOverrideError={saveExploreModelOverrideMutation.isError}
		/>
	);
};

export default AgentSettingsUserAgentsPage;
