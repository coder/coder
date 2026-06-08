import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	aiGatewayPipelinesList,
	aiGatewayPoliciesList,
	createAIGatewayPipelineMutation,
	createAIGatewayPipelineVersionMutation,
	createAIGatewayPolicyMutation,
	createAIGatewayPolicyVersionMutation,
	deleteAIGatewayPipelineMutation,
	deleteAIGatewayPolicyMutation,
	updateAIGatewayPipelineMutation,
	updateAIGatewayPolicyMutation,
} from "#/api/queries/aiGatewayPolicies";
import { aiProvidersList } from "#/api/queries/aiProviders";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import PoliciesPageView from "./PoliciesPageView";

const PoliciesPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const policiesQuery = useQuery(aiGatewayPoliciesList());
	const pipelinesQuery = useQuery(aiGatewayPipelinesList());
	const providersQuery = useQuery(aiProvidersList());
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

	return (
		<RequirePermission isFeatureVisible={permissions.viewAnyAIGatewayPolicy}>
			<title>{pageTitle("AI Gateway Policies")}</title>

			<PoliciesPageView
				policies={policiesQuery.data ?? []}
				pipelines={pipelinesQuery.data ?? []}
				providers={providersQuery.data ?? []}
				isLoading={policiesQuery.isLoading || pipelinesQuery.isLoading}
				error={policiesQuery.error ?? pipelinesQuery.error}
				onCreatePolicy={(request, onSuccess) =>
					createPolicy.mutate(request, { onSuccess })
				}
				isCreating={createPolicy.isPending}
				createError={createPolicy.error}
				onDeletePolicy={deletePolicy.mutate}
				deletePolicyError={deletePolicy.error}
				onEditPolicy={(id, rego, onSuccess) =>
					createPolicyVersion.mutate(
						{ id, request: { rego, activate: true } },
						{ onSuccess },
					)
				}
				isEditing={createPolicyVersion.isPending}
				editError={createPolicyVersion.error}
				onRevertPolicy={(id, versionId, onSuccess) =>
					updatePolicy.mutate(
						{ id, request: { active_version_id: versionId } },
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
				onEditPipeline={(id, policies, onSuccess) =>
					createPipelineVersion.mutate(
						{ id, request: { policies, activate: true } },
						{ onSuccess },
					)
				}
				isEditingPipeline={createPipelineVersion.isPending}
				editPipelineError={createPipelineVersion.error}
				onTogglePipeline={(id, enabled) =>
					updatePipeline.mutate({ id, request: { enabled } })
				}
			/>
		</RequirePermission>
	);
};

export default PoliciesPage;
