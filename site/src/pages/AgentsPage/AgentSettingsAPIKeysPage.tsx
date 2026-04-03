import type { FC } from "react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	deleteUserChatProviderKey,
	upsertUserChatProviderKey,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import { AgentSettingsAPIKeysPageView } from "./AgentSettingsAPIKeysPageView";

const incrementResetToken = (
	current: Record<string, number>,
	providerConfigId: string,
) => ({
	...current,
	[providerConfigId]: (current[providerConfigId] ?? 0) + 1,
});

const AgentSettingsAPIKeysPage: FC = () => {
	const queryClient = useQueryClient();
	const [providerPanelResetTokens, setProviderPanelResetTokens] = useState<
		Record<string, number>
	>({});

	const providersQuery = useQuery(userChatProviderConfigs());
	const modelsQuery = useQuery(chatModelConfigs());

	const upsertMutationOptions = upsertUserChatProviderKey(queryClient);
	const upsertMutation = useMutation({
		...upsertMutationOptions,
		onSuccess: async (_data, variables) => {
			await upsertMutationOptions.onSuccess?.();
			setProviderPanelResetTokens((current) =>
				incrementResetToken(current, variables.providerConfigId),
			);
			toast.success("API key saved.");
		},
		onError: (mutationError) => {
			toast.error(getErrorMessage(mutationError, "Error saving API key."), {
				description: getErrorDetail(mutationError),
			});
		},
	});

	const deleteMutationOptions = deleteUserChatProviderKey(queryClient);
	const deleteMutation = useMutation({
		...deleteMutationOptions,
		onSuccess: async (_data, variables) => {
			await deleteMutationOptions.onSuccess?.();
			setProviderPanelResetTokens((current) =>
				incrementResetToken(current, variables),
			);
			toast.success("API key removed.");
		},
		onError: (mutationError) => {
			toast.error(getErrorMessage(mutationError, "Error removing API key."), {
				description: getErrorDetail(mutationError),
			});
		},
	});

	const providerItems = (providersQuery.data ?? []).map((provider) => ({
		provider,
		renderKey: `${provider.provider_id}-${provider.has_user_api_key}-${providerPanelResetTokens[provider.provider_id] ?? 0}`,
		isSaving:
			upsertMutation.isPending &&
			upsertMutation.variables?.providerConfigId === provider.provider_id,
		isRemoving:
			deleteMutation.isPending &&
			deleteMutation.variables === provider.provider_id,
	}));

	return (
		<AgentSettingsAPIKeysPageView
			error={providersQuery.error}
			isLoading={providersQuery.isLoading}
			providerItems={providerItems}
			models={modelsQuery.data ?? []}
			isModelsLoading={modelsQuery.isLoading}
			areModelsUnavailable={Boolean(modelsQuery.error)}
			onSave={(providerConfigId, apiKey) => {
				upsertMutation.mutate({
					providerConfigId,
					req: { api_key: apiKey },
				});
			}}
			onRemove={(providerConfigId) => {
				deleteMutation.mutate(providerConfigId);
			}}
		/>
	);
};

export default AgentSettingsAPIKeysPage;
