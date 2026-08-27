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
import { useOrganizationChatModels } from "./hooks/useOrganizationChatModels";
import {
	organizationsWithEnabledChatModels,
	resolveModelSelector,
} from "./utils/modelOptions";

const AgentSettingsUserAgentsPage: FC = () => {
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	// Match the compaction page dropdown: list only organizations with at
	// least one enabled chat model.
	const organizationOptions = organizationsWithEnabledChatModels(
		organizations,
		organizationModels.models,
	);
	const defaultOrganizationId = getDefaultOrganizationId(organizations);
	const [selectedOrganizationId, setSelectedOrganizationId] = useState(
		defaultOrganizationId,
	);
	const selectedOrganization =
		organizationOptions.find(
			(organization) => organization.id === selectedOrganizationId,
		) ??
		organizationOptions.find(
			(organization) => organization.id === defaultOrganizationId,
		) ??
		organizationOptions[0] ??
		// No organization has enabled models (still loading, or none are
		// configured). Fall back so the model-free default modes can still
		// replace a stale saved model override.
		organizations.find(
			(organization) => organization.id === defaultOrganizationId,
		) ??
		organizations[0];
	const organizationId = selectedOrganization?.id ?? "";
	// Remount on organization change so mutation state (pending saves,
	// errors) from one organization never renders on another's form.
	return (
		<AgentSettingsUserAgentsPageContent
			key={organizationId}
			organizations={organizationOptions}
			selectedOrganization={selectedOrganization}
			organizationId={organizationId}
			isLoadingOrganizationModels={organizationModels.isLoading}
			onSelectOrganization={(organization) =>
				setSelectedOrganizationId(organization.id)
			}
		/>
	);
};

interface AgentSettingsUserAgentsPageContentProps {
	organizations: readonly TypesGen.Organization[];
	selectedOrganization: TypesGen.Organization | undefined;
	organizationId: string;
	isLoadingOrganizationModels: boolean;
	onSelectOrganization: (organization: TypesGen.Organization) => void;
}

const AgentSettingsUserAgentsPageContent: FC<
	AgentSettingsUserAgentsPageContentProps
> = ({
	organizations,
	selectedOrganization,
	organizationId,
	isLoadingOrganizationModels,
	onSelectOrganization,
}) => {
	const queryClient = useQueryClient();
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
	// Organization filtering needs every organization's models, so stay in
	// the loading state until they all settle.
	const isLoadingModels = isModelCatalogLoading || isLoadingOrganizationModels;
	const hasNoOrganizationModels =
		organizationId !== "" &&
		!isLoadingModels &&
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
			onSelectOrganization={onSelectOrganization}
			models={organizationModelConfigs}
			modelsError={modelsQuery.error}
			isLoadingModels={isLoadingModels}
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
