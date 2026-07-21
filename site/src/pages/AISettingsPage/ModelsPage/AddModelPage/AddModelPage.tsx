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
import { AIResourceOrganizationSelector } from "#/components/AIResourceOrganizationSelector/AIResourceOrganizationSelector";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import {
	canManageProviderModels,
	deriveProviderStates,
} from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import AddModelPageView from "./AddModelPageView";

const AddModelPage: FC = () => {
	const { organization, permissions: organizationPermissions } =
		useAIResourceOrganization();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [searchParams] = useSearchParams();
	const providerKey = searchParams.get("provider") ?? "";
	const duplicateId = searchParams.get("duplicate");

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs(organization.name));
	const modelCatalogQuery = useQuery(chatModels(organization.name));

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
		<RequirePermission
			isFeatureVisible={Boolean(organizationPermissions?.createModel)}
		>
			<title>{pageTitle("Add model", "AI Settings")}</title>
			<AIResourceOrganizationSelector />

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
						const created = await createMutation.mutateAsync({
							organization: organization.name,
							req,
						});
						toast.success(
							`Model "${created.display_name || created.model}" added.`,
						);
						const next = new URLSearchParams(searchParams);
						next.delete("provider");
						next.delete("duplicate");
						await navigate(
							`/ai/settings/models/${created.id}?${next.toString()}`,
						);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add model."));
					}
				}}
			/>
		</RequirePermission>
	);
};

export default AddModelPage;
