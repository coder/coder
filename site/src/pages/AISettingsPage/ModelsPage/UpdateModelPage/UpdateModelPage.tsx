import { type FC, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	deleteChatModelConfig,
	updateChatModelConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import UpdateModelPageView from "./UpdateModelPageView";

const UpdateModelPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { modelId } = useParams<{ modelId: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const updateMutation = useMutation(updateChatModelConfig(queryClient));
	const deleteMutation = useMutation(deleteChatModelConfig(queryClient));

	const providerStates = useMemo(
		() =>
			deriveProviderStates(
				modelConfigsQuery.data ?? [],
				providerConfigsQuery.data,
				modelCatalogQuery.data,
			),
		[modelConfigsQuery.data, providerConfigsQuery.data, modelCatalogQuery.data],
	);

	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;

	const model = modelConfigsQuery.data?.find((m) => m.id === modelId);
	const currentDefaultModel = modelConfigsQuery.data?.find((m) => m.is_default);
	const [providerKeyOverride, setProviderKeyOverride] = useState<string | null>(
		null,
	);
	const selectedProviderState =
		(providerKeyOverride
			? providerStates.find((ps) => ps.key === providerKeyOverride)
			: undefined) ??
		providerStates.find((ps) =>
			ps.modelConfigs.some((m) => m.id === modelId),
		) ??
		null;

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			{!modelId ? (
				<Navigate to="/ai/settings/models" replace />
			) : isLoading ? (
				<>
					<title>{pageTitle("Loading...", "AI Settings")}</title>
					<Loader fullscreen />
				</>
			) : !model ? (
				<Navigate to="/ai/settings/models" replace />
			) : (
				<UpdateModelPageView
					model={model}
					currentDefaultModel={currentDefaultModel}
					providerStates={providerStates}
					selectedProviderState={selectedProviderState}
					onProviderChange={setProviderKeyOverride}
					isSaving={updateMutation.isPending}
					isDeleting={deleteMutation.isPending}
					onUpdateModel={async (id, req) => {
						try {
							const updated = await updateMutation.mutateAsync({
								modelConfigId: id,
								req,
							});
							toast.success(
								`Model "${updated.display_name || updated.model}" updated.`,
							);
							await navigate("/ai/settings/models");
						} catch (error) {
							toast.error(getErrorMessage(error, "Failed to update model."));
						}
					}}
					onDeleteModel={async (id) => {
						try {
							await deleteMutation.mutateAsync(id);
							toast.success(
								`Model "${model.display_name || model.model}" deleted.`,
							);
							await navigate("/ai/settings/models", { replace: true });
						} catch (error) {
							toast.error(getErrorMessage(error, "Failed to delete model."));
						}
					}}
					onDuplicate={() => {
						if (!selectedProviderState) return;
						void navigate(
							`/ai/settings/models/add?provider=${encodeURIComponent(
								selectedProviderState.key,
							)}&duplicate=${encodeURIComponent(model.id)}`,
						);
					}}
					onToggleEnabled={(enabled) => {
						updateMutation.mutate(
							{ modelConfigId: model.id, req: { enabled } },
							{
								onSuccess: () => {
									toast.success(
										`Model "${model.display_name || model.model}" ${
											enabled ? "enabled" : "disabled"
										}.`,
									);
								},
								onError: (error) => {
									toast.error(
										getErrorMessage(error, "Failed to update model."),
									);
								},
							},
						);
					}}
				/>
			)}
		</RequirePermission>
	);
};

export default UpdateModelPage;
