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
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const AgentSettingsUserAgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
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

	// Personal overrides still write global override keys, so only the
	// default-org model rows are valid targets. Non-default-org copies
	// must not be offered here.
	const defaultOrganizationId = organizations.find((org) => org.is_default)?.id;
	const defaultOrgModelConfigs = (modelConfigsQuery.data ?? []).filter(
		(config) => config.organization_id === defaultOrganizationId,
	);
	const hasDefaultOrgModels =
		defaultOrganizationId !== undefined && defaultOrgModelConfigs.length > 0;

	const filteredModelConfigsQuery = {
		data: defaultOrgModelConfigs,
		isLoading: modelConfigsQuery.isLoading,
	};
	const { options: modelOptions, isModelCatalogLoading } = resolveModelSelector(
		filteredModelConfigsQuery,
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
			modelConfigs={defaultOrgModelConfigs}
			modelConfigsError={modelConfigsError}
			isLoadingModels={isModelCatalogLoading}
			hasNoDefaultOrgModels={
				!modelConfigsQuery.isLoading && !hasDefaultOrgModels
			}
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
