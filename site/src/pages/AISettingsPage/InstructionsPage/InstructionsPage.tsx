import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import {
	chatPlanModeInstructions,
	chatSystemPrompt,
	updateChatPlanModeInstructions,
	updateChatSystemPrompt,
} from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { InstructionsPageView } from "./InstructionsPageView";
import { OrganizationInstructions } from "./OrganizationInstructions";
import { readableInstructionsOrganizations } from "./readableInstructionsOrganizations";

const InstructionsPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const organizationPermissionsQuery = useQuery(
		organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
	);
	const readableOrganizations = readableInstructionsOrganizations(
		organizations,
		organizationPermissionsQuery.data,
	);
	const organizationSelection = selectModelOrganization(
		readableOrganizations,
		searchParams.get(modelOrganizationSearchParam),
	);
	const activeOrganization = organizationSelection.organization;

	const systemPromptQuery = useQuery({
		...chatSystemPrompt(),
		enabled: permissions.editDeploymentConfig,
	});
	const planModeInstructionsQuery = useQuery({
		...chatPlanModeInstructions(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveSystemPromptMutation = useMutation(
		updateChatSystemPrompt(queryClient),
	);
	const savePlanModeInstructionsMutation = useMutation(
		updateChatPlanModeInstructions(queryClient),
	);

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig ||
				readableOrganizations.length > 0 ||
				organizationPermissionsQuery.isLoading ||
				organizationPermissionsQuery.error != null
			}
		>
			<title>{pageTitle("Instructions", "AI Settings")}</title>

			<InstructionsPageView
				canEditDeploymentConfig={permissions.editDeploymentConfig}
				systemPromptData={systemPromptQuery.data}
				planModeInstructionsData={planModeInstructionsQuery.data}
				deploymentInstructionsError={
					systemPromptQuery.error ?? planModeInstructionsQuery.error
				}
				onSaveSystemPrompt={saveSystemPromptMutation.mutateAsync}
				onSavePlanModeInstructions={
					savePlanModeInstructionsMutation.mutateAsync
				}
				onResetSystemPromptSave={saveSystemPromptMutation.reset}
				onResetPlanModeInstructionsSave={savePlanModeInstructionsMutation.reset}
				isSaving={
					saveSystemPromptMutation.isPending ||
					savePlanModeInstructionsMutation.isPending
				}
				isSaveSystemPromptError={saveSystemPromptMutation.isError}
				isSavePlanModeInstructionsError={
					savePlanModeInstructionsMutation.isError
				}
				organization={activeOrganization}
				organizations={readableOrganizations}
				onSelectOrganization={(organization) => {
					const next = new URLSearchParams(searchParams);
					next.set(modelOrganizationSearchParam, organization.name);
					setSearchParams(next);
				}}
				requestedOrganizationDenied={
					organizationSelection.requestedOrganizationDenied
				}
				isOrganizationAccessLoading={organizationPermissionsQuery.isLoading}
				organizationAccessError={organizationPermissionsQuery.error}
				organizationInstructions={
					activeOrganization && (
						<OrganizationInstructions
							key={activeOrganization.id}
							organization={activeOrganization}
							canEdit={
								!organizationSelection.requestedOrganizationDenied &&
								Boolean(
									organizationPermissionsQuery.data?.[activeOrganization.id]
										?.editChatModelConfigs,
								)
							}
						/>
					)
				}
			/>
		</RequirePermission>
	);
};

export default InstructionsPage;
