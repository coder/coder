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
import type * as TypesGen from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsUserAgentsPageView } from "./AgentSettingsUserAgentsPageView";
import { resolveModelSelector } from "./utils/modelOptions";

const organizationSearchParam = "org";

const overrideSaveToast = {
	root: {
		success: "Root agent model saved.",
		error: "Failed to save root agent model.",
	},
	general: {
		success: "General subagent model saved.",
		error: "Failed to save general subagent model.",
	},
	explore: {
		success: "Explore subagent model saved.",
		error: "Failed to save Explore subagent model.",
	},
} as const satisfies Record<
	TypesGen.ChatPersonalModelOverrideContext,
	{ success: string; error: string }
>;

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
			toast.success(overrideSaveToast[variables.context].success);
		},
		onError: (error, variables) => {
			toast.error(
				getErrorMessage(error, overrideSaveToast[variables.context].error),
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
