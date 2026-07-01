import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	groupAIBudget,
	patchGroup,
	saveGroupAIBudget,
} from "#/api/queries/groups";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { dollarsToMicros, microsToDollars } from "#/utils/currency";
import type { GroupPageOutletContext } from "./GroupPage";
import GroupSettingsPageView from "./GroupSettingsPageView";

const budgetFromInput = (dollars: string): number | null =>
	dollars.trim() === "" ? null : dollarsToMicros(dollars);

const GroupSettingsPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const { group: groupData } = useOutletContext<GroupPageOutletContext>();
	const queryClient = useQueryClient();
	const patchGroupMutation = useMutation(patchGroup(queryClient, organization));
	const navigate = useNavigate();

	const { experiments } = useDashboard();
	// TODO(AIGOV-443): remove the ai-gateway-cost-control experiment gate once
	// the cost-control feature is stable.
	const aibridgeVisible =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");
	const budgetQuery = useQuery({
		...groupAIBudget(groupData.id),
		enabled: aibridgeVisible,
	});
	const saveBudgetMutation = useMutation(
		saveGroupAIBudget(queryClient, groupData.id),
	);

	if (aibridgeVisible && budgetQuery.isLoading) {
		return (
			<div className="flex items-center justify-center p-10">
				<Spinner loading className="size-6" />
			</div>
		);
	}
	if (aibridgeVisible && budgetQuery.error) {
		return <ErrorAlert error={budgetQuery.error} />;
	}

	const currentBudgetMicros = budgetQuery.data?.spend_limit_micros ?? null;
	const initialBudgetDollars =
		currentBudgetMicros !== null ? microsToDollars(currentBudgetMicros) : null;
	const isUpdating =
		patchGroupMutation.isPending || saveBudgetMutation.isPending;

	return (
		<GroupSettingsPageView
			onCancel={() => navigate("..")}
			onSubmit={async (data) => {
				const { monthly_budget_per_member, ...groupFields } = data;
				try {
					await patchGroupMutation.mutateAsync({
						groupId: groupData.id,
						...groupFields,
						add_users: [],
						remove_users: [],
					});
				} catch (error) {
					toast.error(
						getErrorMessage(error, `Failed to update group "${groupName}".`),
						{ description: getErrorDetail(error) },
					);
					return;
				}

				const next = budgetFromInput(monthly_budget_per_member);
				if (aibridgeVisible && next !== currentBudgetMicros) {
					try {
						await saveBudgetMutation.mutateAsync(next);
					} catch (error) {
						toast.error(
							getErrorMessage(error, "Failed to update the AI budget."),
							{ description: getErrorDetail(error) },
						);
						return;
					}
				}

				navigate(`/organizations/${organization}/groups/${data.name}`);
			}}
			group={groupData}
			showAISettings={aibridgeVisible}
			initialBudgetDollars={initialBudgetDollars}
			formErrors={undefined}
			isUpdating={isUpdating}
		/>
	);
};

export default GroupSettingsPage;
