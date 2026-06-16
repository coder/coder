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
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { dollarsToMicros, microsToDollars } from "#/utils/currency";
import type { GroupPageOutletContext } from "./GroupPage";
import GroupSettingsPageView from "./GroupSettingsPageView";

// Empty is uncapped (no budget row); otherwise the budget in micros.
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

	// Budget routes are gated on aibridge; useFeatureVisibility is {} unlicensed.
	const aibridgeVisible = Boolean(useFeatureVisibility().aibridge);
	const budgetQuery = useQuery({
		...groupAIBudget(groupData.id),
		enabled: aibridgeVisible,
	});
	const saveBudgetMutation = useMutation(
		saveGroupAIBudget(queryClient, groupData.id),
	);

	// Load the budget before rendering so the form initializes with it.
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

					// Save only when the budget changed (0 disables, empty uncaps).
					const next = budgetFromInput(monthly_budget_per_member);
					if (aibridgeVisible && next !== currentBudgetMicros) {
						await saveBudgetMutation.mutateAsync(next);
					}

					navigate(`/organizations/${organization}/groups/${data.name}`);
				} catch (error) {
					toast.error(
						getErrorMessage(error, `Failed to update group "${groupName}".`),
						{ description: getErrorDetail(error) },
					);
				}
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
