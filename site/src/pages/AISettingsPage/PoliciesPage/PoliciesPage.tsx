import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	aiGatewayGuardrailsList,
	createAIGatewayGuardrailMutation,
	createAIGatewayGuardrailVersionMutation,
	deleteAIGatewayGuardrailMutation,
	updateAIGatewayGuardrailMutation,
} from "#/api/queries/aiGatewayGuardrails";
import {
	aiGatewayPipelinesList,
	aiGatewayPoliciesList,
	createAIGatewayPipelineMutation,
	createAIGatewayPipelineVersionMutation,
	createAIGatewayPolicyMutation,
	createAIGatewayPolicyVersionMutation,
	deleteAIGatewayPipelineMutation,
	deleteAIGatewayPolicyMutation,
	updateAIGatewayPipelineMemberMutation,
	updateAIGatewayPipelineMutation,
	updateAIGatewayPolicyMutation,
} from "#/api/queries/aiGatewayPolicies";
import { aiProvidersList } from "#/api/queries/aiProviders";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { GuardrailsSection } from "./GuardrailsSection";
import PoliciesPageView from "./PoliciesPageView";

const PoliciesPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const policiesQuery = useQuery(aiGatewayPoliciesList());
	const pipelinesQuery = useQuery(aiGatewayPipelinesList());
	const providersQuery = useQuery(aiProvidersList());
	const guardrailsQuery = useQuery(aiGatewayGuardrailsList());
	const createGuardrail = useMutation(
		createAIGatewayGuardrailMutation(queryClient),
	);
	const createGuardrailVersion = useMutation(
		createAIGatewayGuardrailVersionMutation(queryClient),
	);
	const deleteGuardrail = useMutation(
		deleteAIGatewayGuardrailMutation(queryClient),
	);
	const updateGuardrail = useMutation(
		updateAIGatewayGuardrailMutation(queryClient),
	);
	const createPolicy = useMutation(createAIGatewayPolicyMutation(queryClient));
	const deletePolicy = useMutation(deleteAIGatewayPolicyMutation(queryClient));
	const createPipeline = useMutation(
		createAIGatewayPipelineMutation(queryClient),
	);
	const deletePipeline = useMutation(
		deleteAIGatewayPipelineMutation(queryClient),
	);
	const createPipelineVersion = useMutation(
		createAIGatewayPipelineVersionMutation(queryClient),
	);
	const updatePolicy = useMutation(updateAIGatewayPolicyMutation(queryClient));
	const createPolicyVersion = useMutation(
		createAIGatewayPolicyVersionMutation(queryClient),
	);
	const updatePipeline = useMutation(
		updateAIGatewayPipelineMutation(queryClient),
	);
	const updatePipelineMember = useMutation(
		updateAIGatewayPipelineMemberMutation(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.viewAnyAIGatewayPolicy}>
			<title>{pageTitle("AI Gateway Policies")}</title>

			<div className="flex flex-col gap-8">
				<PoliciesPageView
					policies={policiesQuery.data ?? []}
					pipelines={pipelinesQuery.data ?? []}
					providers={providersQuery.data ?? []}
					guardrails={guardrailsQuery.data ?? []}
					isLoading={policiesQuery.isLoading || pipelinesQuery.isLoading}
					error={policiesQuery.error ?? pipelinesQuery.error}
					onCreatePolicy={(request, onSuccess) =>
						createPolicy.mutate(request, { onSuccess })
					}
					isCreating={createPolicy.isPending}
					createError={createPolicy.error}
					onDeletePolicy={deletePolicy.mutate}
					deletePolicyError={deletePolicy.error}
					onEditPolicy={(id, rego, promote, onSuccess) =>
						createPolicyVersion.mutate(
							{ id, request: { rego, activate: true, promote } },
							{ onSuccess },
						)
					}
					isEditing={createPolicyVersion.isPending}
					editError={createPolicyVersion.error}
					onRevertPolicy={(id, versionId, promote, onSuccess) =>
						updatePolicy.mutate(
							{ id, request: { active_version_id: versionId, promote } },
							{ onSuccess },
						)
					}
					isReverting={updatePolicy.isPending}
					revertError={updatePolicy.error}
					onCreatePipeline={(request, onSuccess) =>
						createPipeline.mutate(request, { onSuccess })
					}
					isCreatingPipeline={createPipeline.isPending}
					createPipelineError={createPipeline.error}
					onDeletePipeline={deletePipeline.mutate}
					deletePipelineError={deletePipeline.error}
					onEditPipeline={(id, policies, guardrails, onSuccess) =>
						// Editing a pipeline's membership mints an unpromoted draft
						// version (activate: false). Promotion is a separate,
						// deliberate action via onPromotePipeline; nothing goes live
						// until the operator promotes the staged version.
						createPipelineVersion.mutate(
							{ id, request: { policies, guardrails, activate: false } },
							{ onSuccess },
						)
					}
					isEditingPipeline={createPipelineVersion.isPending}
					editPipelineError={createPipelineVersion.error}
					onTogglePipeline={(id, enabled) =>
						updatePipeline.mutate({ id, request: { enabled } })
					}
					onToggleMember={(id, request) =>
						updatePipelineMember.mutate({ id, request })
					}
					onPromotePipeline={(id, versionId, onSuccess) =>
						updatePipeline.mutate(
							{ id, request: { active_version_id: versionId } },
							{ onSuccess },
						)
					}
					isPromoting={updatePipeline.isPending}
					promoteError={updatePipeline.error}
				/>

				<GuardrailsSection
					guardrails={guardrailsQuery.data ?? []}
					isLoading={guardrailsQuery.isLoading}
					error={guardrailsQuery.error}
					onCreate={(request, onSuccess) =>
						createGuardrail.mutate(request, { onSuccess })
					}
					isCreating={createGuardrail.isPending}
					createError={createGuardrail.error}
					onEdit={(id, request, onSuccess) =>
						createGuardrailVersion.mutate({ id, request }, { onSuccess })
					}
					isEditing={createGuardrailVersion.isPending}
					editError={createGuardrailVersion.error}
					onDelete={deleteGuardrail.mutate}
					deleteError={deleteGuardrail.error}
					onToggle={(id, enabled) =>
						updateGuardrail.mutate({ id, request: { enabled } })
					}
				/>
			</div>
		</RequirePermission>
	);
};

export default PoliciesPage;
