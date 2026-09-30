import { useQuery } from "react-query";
import { chatAutomations } from "#/api/queries/chatAutomations";
import { useDashboard } from "#/modules/dashboard/useDashboard";

export type ChatAutomationNames = {
	names: ReadonlyMap<string, string>;
	// State of the automations list: while "loading" (including refetches)
	// a missing name may still resolve, and "error" means the list failed.
	status: "loading" | "error" | "settled";
};

/**
 * Maps automation IDs to names for labeling automation input. Deleted or
 * unreadable automations are absent from the map.
 */
export const useChatAutomationNames = (
	organizationId: string | undefined,
	hasAutomationInput: boolean,
): ChatAutomationNames => {
	const { experiments } = useDashboard();
	const automationsQuery = useQuery({
		...chatAutomations(organizationId ?? ""),
		enabled:
			Boolean(organizationId) &&
			hasAutomationInput &&
			experiments.includes("chat-automations"),
	});
	return {
		names: new Map(
			automationsQuery.data?.map((automation) => [
				automation.id,
				automation.name,
			]),
		),
		status: automationsQuery.isFetching
			? "loading"
			: automationsQuery.isError
				? "error"
				: "settled",
	};
};
