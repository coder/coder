import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModels,
	updateUserChatPersonalModelOverride,
	userChatPersonalModelOverrides,
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
	const defaultOrganizationId = getDefaultOrganizationId(organizations);
	const [selectedOrganizationId, setSelectedOrganizationId] = useState(
		defaultOrganizationId,
	);
	const selectedOrganization =
		organizations.find(
			(organization) => organization.id === selectedOrganizationId,
		) ??
		organizations.find(
			(organization) => organization.id === defaultOrganizationId,
		) ??
		organizations[0];
	const organizationId = selectedOrganization?.id ?? "";
	const overridesQuery = useQuery(
		userChatPersonalModelOverrides(organizationId),
	);
	const modelsQuery = useQuery(chatModels(organizationId));
	const saveRootModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient, organizationId),
	);
	const saveGeneralModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient, organizationId),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateUserChatPersonalModelOverride(queryClient, organizationId),
	);

	const organizationModelConfigs = modelsQuery.data?.models ?? [];

	const { options: modelOptions, isModelCatalogLoading } = resolveModelSelector(
		organizationId,
		modelsQuery,
	);
	const hasNoOrganizationModels =
		organizationId !== "" &&
		!modelsQuery.isLoading &&
		modelsQuery.error === null &&
		modelsQuery.data !== undefined &&
		modelOptions.length === 0;

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
			organizations={organizations}
			selectedOrganization={selectedOrganization}
			onSelectOrganization={(organization) =>
				setSelectedOrganizationId(organization.id)
			}
			models={organizationModelConfigs}
			modelsError={modelsQuery.error}
			isLoadingModels={isModelCatalogLoading}
			isOrganizationUnresolved={organizationId === ""}
			hasNoOrganizationModels={hasNoOrganizationModels}
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
