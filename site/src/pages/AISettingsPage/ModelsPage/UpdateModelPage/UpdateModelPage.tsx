import { type FC, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useLocation, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatModelConfig,
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	deleteChatModelConfig,
	updateChatModelConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import { deriveProviderStates } from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import UpdateModelPageView from "./UpdateModelPageView";

const UpdateModelPage: FC = () => {
	const { organization, permissions: organizationPermissions } =
		useAIResourceOrganization();
	const { modelId } = useParams<{ modelId: string }>();
	const navigate = useNavigate();
	const location = useLocation();
	const queryClient = useQueryClient();

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelQuery = useQuery({
		...chatModelConfig(modelId ?? ""),
		enabled: Boolean(modelId),
	});
	const modelConfigsQuery = useQuery(chatModelConfigs(organization.name));
	const modelCatalogQuery = useQuery(chatModels(organization.name));

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
		modelQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;

	const model =
		modelQuery.data?.organization_id === organization.id
			? modelQuery.data
			: undefined;
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
		<RequirePermission
			isFeatureVisible={Boolean(organizationPermissions?.editModels)}
		>
			{!modelId ? (
				<Navigate
					to={{ pathname: "/ai/settings/models", search: location.search }}
					replace
				/>
			) : isLoading ? (
				<>
					<title>{pageTitle("Loading...", "AI Settings")}</title>
					<Loader fullscreen />
				</>
			) : !model ? (
				<Navigate
					to={{ pathname: "/ai/settings/models", search: location.search }}
					replace
				/>
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
								organization: organization.name,
								modelConfigId: id,
								req,
							});
							toast.success(
								`Model "${updated.display_name || updated.model}" updated.`,
							);
							await navigate({
								pathname: "/ai/settings/models",
								search: location.search,
							});
						} catch (error) {
							toast.error(getErrorMessage(error, "Failed to update model."));
						}
					}}
					onDeleteModel={
						organizationPermissions.deleteModels
							? async (id) => {
									try {
										await deleteMutation.mutateAsync({
											organization: organization.name,
											modelConfigId: id,
										});
										toast.success(
											`Model "${model.display_name || model.model}" deleted.`,
										);
										await navigate(
											{
												pathname: "/ai/settings/models",
												search: location.search,
											},
											{ replace: true },
										);
									} catch (error) {
										toast.error(
											getErrorMessage(error, "Failed to delete model."),
										);
									}
								}
							: undefined
					}
					sharingPath={`/ai/settings/models/${model.id}/sharing`}
					onDuplicate={() => {
						if (!selectedProviderState) return;
						const params = new URLSearchParams(location.search);
						params.set("provider", selectedProviderState.key);
						params.set("duplicate", model.id);
						void navigate({
							pathname: "/ai/settings/models/add",
							search: params.toString(),
						});
					}}
					onToggleEnabled={(enabled) => {
						updateMutation.mutate(
							{
								organization: organization.name,
								modelConfigId: model.id,
								req: { enabled },
							},
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
