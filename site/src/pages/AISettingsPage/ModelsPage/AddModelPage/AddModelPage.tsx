import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatModels, createChatModel } from "#/api/queries/chats";
import {
	canManageProviderModels,
	deriveProviderStates,
} from "#/modules/aiModels/providerStates";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import {
	organizationAddModelPath,
	organizationModelPath,
	splitModelQueryErrors,
	useOrganizationModels,
} from "../organizationModels";
import AddModelPageView from "./AddModelPageView";

const AddModelPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [searchParams] = useSearchParams();
	const providerKey = searchParams.get("provider") ?? "";
	const duplicateId = searchParams.get("duplicate");
	const { organization, permissions, requestedOrganizationDenied } =
		useOrganizationModels();

	const organizationModelsQuery = useQuery(chatModels(organization.id));
	const createMutation = useMutation(createChatModel(queryClient));
	const models = organizationModelsQuery.data?.models ?? [];
	const providerStates = deriveProviderStates(
		models,
		organizationModelsQuery.data?.providers ?? [],
	);
	const isLoading = organizationModelsQuery.isLoading;
	const { loadError, refetchError } = splitModelQueryErrors(
		organizationModelsQuery,
	);

	if (requestedOrganizationDenied) {
		return (
			<>
				<title>{pageTitle("Add model", "AI Settings")}</title>
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	const selectedProviderState = providerKey
		? (providerStates.find((ps) => ps.key === providerKey) ?? null)
		: (providerStates.find(canManageProviderModels) ?? null);
	const duplicateSourceModel = duplicateId
		? models.find((m) => m.id === duplicateId)
		: undefined;
	const currentDefaultModel = models.find((m) => m.is_default);

	return (
		<>
			<title>{pageTitle("Add model", "AI Settings")}</title>

			<RequirePermission
				isFeatureVisible={permissions?.createChatModelConfigs ?? false}
			>
				<AddModelPageView
					isLoading={isLoading}
					loadError={loadError}
					refetchError={refetchError}
					providerStates={providerStates}
					selectedProviderState={selectedProviderState}
					duplicateSourceModel={duplicateSourceModel}
					currentDefaultModel={currentDefaultModel}
					isSaving={createMutation.isPending}
					onProviderChange={(key) => {
						const next = new URLSearchParams(searchParams);
						next.set("provider", key);
						void navigate(organizationAddModelPath(organization, next), {
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
							await navigate(organizationModelPath(organization, created.id));
						} catch (error) {
							toast.error(getErrorMessage(error, "Failed to add model."));
						}
					}}
				/>
			</RequirePermission>
		</>
	);
};

export default AddModelPage;
