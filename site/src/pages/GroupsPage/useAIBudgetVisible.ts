import { useQuery } from "react-query";
import type { UserAISpend } from "#/api/api";
import { meAISpend } from "#/api/queries/users";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";

/** Whether the AI cost-control UI should render, and the viewer's AI spend. */
export function useAIBudgetVisible(): {
	visible: boolean;
	aiSpend: UserAISpend | undefined;
} {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): remove the ai-gateway-cost-control experiment gate once
	// the cost-control feature is stable.
	const visible =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");
	const { data: aiSpend } = useQuery({ ...meAISpend(), enabled: visible });
	return { visible, aiSpend };
}
