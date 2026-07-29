import { type FC, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatAIProviderCatalog,
	chatModelConfigsByOrganization,
	chatModels,
	createChatModelConfig,
} from "#/api/queries/chats";
import {
	canManageProviderModels,
	deriveProviderStates,
	providerConfigFromCatalogEntry,
} from "#/modules/aiModels/providerStates";
import { pageTitle } from "#/utils/page";
import {
	useOrganizationModels,
	useOrganizationModelsPath,
} from "../organizationModels";
import AddModelPageView from "./AddModelPageView";

const AddModelPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [searchParams] = useSearchParams();
	const providerKey = searchParams.get("provider") ?? "";
	const duplicateId = searchParams.get("duplicate");
	const { organization } = useOrganizationModels();
	const modelsPath = useOrganizationModelsPath();

	const providerCatalogQuery = useQuery(chatAIProviderCatalog());
	const modelConfigsQuery = useQuery(
		chatModelConfigsByOrganization(organization.id),
	);
	const modelCatalogQuery = useQuery(chatModels());

	const createMutation = useMutation(createChatModelConfig(queryClient));

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

	const selectedProviderState = providerKey
		? (providerStates.find((ps) => ps.key === providerKey) ?? null)
		: (providerStates.find(canManageProviderModels) ?? null);
	const duplicateSourceModel = duplicateId
		? modelConfigsQuery.data?.find((m) => m.id === duplicateId)
		: undefined;
	const currentDefaultModel = modelConfigsQuery.data?.find((m) => m.is_default);

	return (
		<>
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
					void navigate(`${modelsPath}/add?${next.toString()}`, {
						replace: true,
					});
				}}
				onCreateModel={async (req) => {
					try {
						const created = await createMutation.mutateAsync({
							organizationId: organization.id,
							req,
						});
						toast.success(
							`Model "${created.display_name || created.model}" added.`,
						);
						await navigate(`${modelsPath}/${created.id}`);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to add model."));
					}
				}}
			/>
		</>
	);
};

export default AddModelPage;
