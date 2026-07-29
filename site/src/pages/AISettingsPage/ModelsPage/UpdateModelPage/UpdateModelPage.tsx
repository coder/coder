import { type FC, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatAIProviderCatalog,
	chatModelConfigsByOrganization,
	chatModels,
	deleteChatModelConfig,
	updateChatModelConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import {
	deriveProviderStates,
	providerConfigFromCatalogEntry,
} from "#/modules/aiModels/providerStates";
import { pageTitle } from "#/utils/page";
import { useOrganizationModels } from "../organizationModels";
import { useOrganizationModelsPath } from "../useOrganizationModelsPath";
import UpdateModelPageView from "./UpdateModelPageView";

const UpdateModelPage: FC = () => {
	const { modelId } = useParams<{ modelId: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization } = useOrganizationModels();
	const modelsPath = useOrganizationModelsPath();

	const providerCatalogQuery = useQuery(chatAIProviderCatalog());
	const modelConfigsQuery = useQuery(
		chatModelConfigsByOrganization(organization.id),
	);
	const modelCatalogQuery = useQuery(chatModels());

	const updateMutation = useMutation(updateChatModelConfig(queryClient));
	const deleteMutation = useMutation(deleteChatModelConfig(queryClient));

	const providerConfigs = providerCatalogQuery.data?.map(
		providerConfigFromCatalogEntry,
	);
	const providerStates = useMemo(
		() =>
			deriveProviderStates(
				modelConfigsQuery.data ?? [],
				providerConfigs,
				modelCatalogQuery.data,
			),
		[modelConfigsQuery.data, providerConfigs, modelCatalogQuery.data],
	);

	const isLoading =
		providerCatalogQuery.isLoading ||
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

	if (!modelId) {
		return <Navigate to={modelsPath} replace />;
	}

	if (isLoading) {
		return (
			<>
				<title>{pageTitle("Loading...", "AI Settings")}</title>
				<Loader fullscreen />
			</>
		);
	}

	if (!model) {
		return <Navigate to={modelsPath} replace />;
	}

	return (
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
					await navigate(modelsPath);
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
					await navigate(modelsPath, { replace: true });
				} catch (error) {
					toast.error(getErrorMessage(error, "Failed to delete model."));
				}
			}}
			onDuplicate={() => {
				if (!selectedProviderState) return;
				void navigate(
					`${modelsPath}/add?provider=${encodeURIComponent(
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
							toast.error(getErrorMessage(error, "Failed to update model."));
						},
					},
				);
			}}
		/>
	);
};

export default UpdateModelPage;
