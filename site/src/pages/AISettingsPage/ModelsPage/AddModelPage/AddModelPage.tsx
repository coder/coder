import { type FC, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	canManageProviderModels,
	deriveProviderStates,
} from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import AddModelPageView from "./AddModelPageView";

const AddModelPage: FC = () => {
	const { permissions } = useAuthenticated();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [searchParams] = useSearchParams();
	const providerKey = searchParams.get("provider") ?? "";
	const duplicateId = searchParams.get("duplicate");

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const createMutation = useMutation(createChatModelConfig(queryClient));

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

	const selectedProviderState = providerKey
		? (providerStates.find((ps) => ps.key === providerKey) ?? null)
		: (providerStates.find(canManageProviderModels) ?? null);
	const duplicateSourceModel = duplicateId
		? modelConfigsQuery.data?.find((m) => m.id === duplicateId)
		: undefined;
	const currentDefaultModel = modelConfigsQuery.data?.find((m) => m.is_default);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Add model", "AI Settings")}</title>

			<AddModelPageView
				isLoading={isLoading}
				providerStates={providerStates}
				selectedProviderState={selectedProviderState}
				duplicateSourceModel={duplicateSourceModel}
				currentDefaultModel={currentDefaultModel}
				isSaving={createMutation.isPending}
				onProviderChange={(key) => {
					const next = new URLSearchParams(searchParams);
					next.set("provider", key);
					void navigate(`/ai/settings/models/add?${next.toString()}`, {
						replace: true,
					});
				}}
				onCreateModel={async (req) => {
					try {
						const created = await createMutation.mutateAsync(req);
						toast.success(
							`Model "${created.display_name || created.model}" added.`,
						);
						await navigate(`/ai/settings/models/${created.id}`);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add model."));
					}
				}}
			/>
		</RequirePermission>
	);
};

export default AddModelPage;
