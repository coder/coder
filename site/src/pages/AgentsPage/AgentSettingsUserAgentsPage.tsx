import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelAvailability,
	updateUserChatPersonalModelOverride,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const AgentSettingsUserAgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const overridesQuery = useQuery(userChatPersonalModelOverrides());
	const defaultOrganizationId = getDefaultOrganizationId(organizations);
	const availableModelsQuery = useQuery(
		chatModelAvailability(defaultOrganizationId),
	);
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

	const defaultOrgModelConfigs = availableModelsQuery.data?.models ?? [];
	const hasDefaultOrgModels = defaultOrgModelConfigs.length > 0;

	const { options: modelOptions, isModelCatalogLoading } = resolveModelSelector(
		defaultOrganizationId,
		availableModelsQuery,
		providerConfigsQuery,
	);

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
			models={defaultOrgModelConfigs}
			modelsError={availableModelsQuery.error}
			isLoadingModels={isModelCatalogLoading}
			isDefaultOrganizationUnresolved={defaultOrganizationId === ""}
			hasNoDefaultOrgModels={
				defaultOrganizationId !== "" &&
				!availableModelsQuery.isLoading &&
				availableModelsQuery.error === null &&
				availableModelsQuery.data !== undefined &&
				!hasDefaultOrgModels
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
