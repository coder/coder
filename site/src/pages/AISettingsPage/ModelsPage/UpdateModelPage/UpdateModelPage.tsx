import { type FC, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { aiModelPrice, upsertAIModelPrices } from "#/api/queries/aiModelPrices";
import { chatProviderConfigs } from "#/api/queries/aiProviders";
import {
	chatModelConfigs,
	chatModels,
	deleteChatModelConfig,
	updateChatModelConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import UpdateModelPageView from "./UpdateModelPageView";

const UpdateModelPage: FC = () => {
	const { permissions } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
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
	const modelProviderState =
		providerStates.find((ps) =>
			ps.modelConfigs.some((m) => m.id === modelId),
		) ?? null;
	const selectedProviderState =
		(providerKeyOverride
			? providerStates.find((ps) => ps.key === providerKeyOverride)
			: undefined) ?? modelProviderState;
	const pricingProvider = modelProviderState?.provider ?? "";
	const pricingModel = model?.model ?? "";
	const isPricingFeatureAvailable = featureVisibility.aibridge;
	const isPricingProviderSupported = pricingProvider !== "openai-compat";
	const canQueryPricing =
		isPricingFeatureAvailable &&
		isPricingProviderSupported &&
		permissions.viewAIModelPrices;
	const modelPricesQuery = useQuery({
		...aiModelPrice(pricingProvider, pricingModel),
		enabled: canQueryPricing && pricingProvider !== "" && pricingModel !== "",
	});
	const modelPricesMutation = useMutation(upsertAIModelPrices(queryClient));

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
					modelPricing={modelPricesQuery.data}
					pricingProvider={modelProviderState?.provider}
					isPricingLoading={canQueryPricing && modelPricesQuery.isLoading}
					isPricingFetching={canQueryPricing && modelPricesQuery.isFetching}
					pricingError={canQueryPricing ? modelPricesQuery.error : undefined}
					isPricingSaving={modelPricesMutation.isPending}
					pricingSaveError={modelPricesMutation.error}
					isPricingFeatureAvailable={isPricingFeatureAvailable}
					canViewPricing={permissions.viewAIModelPrices}
					canEditPricing={
						permissions.viewAIModelPrices && permissions.updateAIModelPrices
					}
					onSavePricing={(price) =>
						new Promise<void>((resolve, reject) => {
							modelPricesMutation.mutate(
								{ prices: [price] },
								{
									onSuccess: () => {
										toast.success("Model pricing updated.");
										resolve();
									},
									onError: (error) => {
										toast.error(
											getErrorMessage(error, "Failed to update model pricing."),
										);
										reject(error);
									},
								},
							);
						})
					}
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
