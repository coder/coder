import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	chatModels,
	updateUserChatPersonalModelOverride,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import type {
	ChatPersonalModelOverrideContext,
	Organization,
} from "#/api/typesGenerated";
import { getOrganizationLabel } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const organizationSearchParam = "org";

const overrideSaveLabel = {
	root: "Root agent model",
	general: "General subagent model",
	explore: "Explore subagent model",
} as const satisfies Record<ChatPersonalModelOverrideContext, string>;

const overrideSaveToast = (
	organizations: readonly Organization[],
	organizationId: string,
	context: ChatPersonalModelOverrideContext,
): { success: string; error: string } => {
	const label = overrideSaveLabel[context];
	const organization =
		organizations.length > 1
			? organizations.find((org) => org.id === organizationId)
			: undefined;
	if (!organization) {
		return {
			success: `${label} saved successfully.`,
			error: `Failed to save ${label}.`,
		};
	}
	const organizationLabel = getOrganizationLabel(organization, organizations);
	return {
		success: `${label} for "${organizationLabel}" saved successfully.`,
		error: `Failed to save ${label} for "${organizationLabel}".`,
	};
};

const AgentSettingsUserAgentsPage: FC = () => {
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const selectedOrganization =
		organizations.find(
			(organization) =>
				organization.name === searchParams.get(organizationSearchParam),
		) ??
		organizations.find((organization) => organization.is_default) ??
		organizations[0];
	const organizationId = selectedOrganization?.id ?? "";
	const queryClient = useQueryClient();

	const overridesQuery = useQuery(
		userChatPersonalModelOverrides(organizationId),
	);
	const modelsQuery = useQuery(chatModels(organizationId));

	const saveOverrideOptions = updateUserChatPersonalModelOverride(queryClient);
	const saveOverride = useMutation({
		...saveOverrideOptions,
		onSuccess: async (data, variables) => {
			await saveOverrideOptions.onSuccess?.(data, variables);
			toast.success(
				overrideSaveToast(
					organizations,
					variables.organizationId,
					variables.context,
				).success,
			);
		},
		onError: (error, variables) => {
			toast.error(
				getErrorMessage(
					error,
					overrideSaveToast(
						organizations,
						variables.organizationId,
						variables.context,
					).error,
				),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	const { options: modelOptions, isModelCatalogLoading } = resolveModelSelector(
		organizationId,
		modelsQuery,
	);
	const saveMatchesOrganization =
		saveOverride.variables?.organizationId === organizationId;

	return (
		<AgentSettingsUserAgentsPageView
			key={organizationId}
			overridesData={overridesQuery.data}
			overridesError={overridesQuery.error}
			onRetryOverrides={() => {
				void overridesQuery.refetch();
			}}
			isRetryingOverrides={overridesQuery.isFetching}
			isLoading={overridesQuery.isLoading || isModelCatalogLoading}
			modelOptions={modelOptions}
			organizations={organizations}
			selectedOrganization={selectedOrganization}
			onSelectOrganization={(organization) => {
				setSearchParams((params) => {
					const next = new URLSearchParams(params);
					next.set(organizationSearchParam, organization.name);
					return next;
				});
			}}
			models={modelsQuery.data?.models ?? []}
			modelsError={modelsQuery.error}
			onSaveOverride={saveOverride.mutate}
			isSaving={saveOverride.isPending && saveMatchesOrganization}
			saveContext={
				saveMatchesOrganization ? saveOverride.variables?.context : undefined
			}
		/>
	);
};

export default AgentSettingsUserAgentsPage;
