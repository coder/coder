import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigsByOrganization,
	chatModels,
	updateUserChatPersonalModelOverride,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import { permittedOrganizations } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const AgentSettingsUserAgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const permittedOrgsQuery = useQuery(
		permittedOrganizations({
			object: { resource_type: "chat" },
			action: "create",
		}),
	);
	const permittedOrgs = permittedOrgsQuery.data ?? organizations;
	const [selectedOrg, setSelectedOrg] = useState<TypesGen.Organization | null>(
		null,
	);
	// Personal overrides are keyed per organization; until the user picks
	// one explicitly, track the default org (today's implicit scope).
	const activeOrg =
		selectedOrg && permittedOrgs.some((org) => org.id === selectedOrg.id)
			? selectedOrg
			: (permittedOrgs.find((org) => org.is_default) ??
				permittedOrgs[0] ??
				null);
	const organizationId = activeOrg?.id ?? "";

	const overridesQuery = useQuery({
		...userChatPersonalModelOverrides(organizationId),
		enabled: organizationId !== "",
	});
	const chatModelsQuery = useQuery(chatModels());
	const modelConfigsQuery = useQuery(
		chatModelConfigsByOrganization(organizationId),
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

	const orgModelConfigs = modelConfigsQuery.data ?? [];
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
			mutation.mutate({ organizationId, context, req }, options);
		};
	};

	return (
		<AgentSettingsUserAgentsPageView
			organizations={permittedOrgs}
			selectedOrganization={activeOrg}
			onOrganizationChange={setSelectedOrg}
			overridesData={overridesQuery.data}
			overridesError={overridesQuery.error}
			onRetryOverrides={() => {
				void overridesQuery.refetch();
			}}
			isRetryingOverrides={overridesQuery.isFetching}
			isLoadingOverrides={overridesQuery.isLoading}
			modelOptions={modelOptions}
			modelConfigs={orgModelConfigs}
			modelConfigsError={modelConfigsError}
			isLoadingModels={isModelCatalogLoading}
			hasNoOrgModels={
				organizationId !== "" &&
				!modelConfigsQuery.isLoading &&
				orgModelConfigs.length === 0
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
