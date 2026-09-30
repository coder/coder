import { useQuery } from "react-query";
import { chatAutomations } from "#/api/queries/chatAutomations";
import { useDashboard } from "#/modules/dashboard/useDashboard";

/**
 * Maps automation IDs to names for labeling automation input. Deleted or
 * unreadable automations are absent from the map.
 */
export const useChatAutomationNames = (
	organizationId: string | undefined,
	hasAutomationInput: boolean,
): ReadonlyMap<string, string> => {
	const { experiments } = useDashboard();
	const automationsQuery = useQuery({
		...chatAutomations(organizationId ?? ""),
		enabled:
			Boolean(organizationId) &&
			hasAutomationInput &&
			experiments.includes("chat-automations"),
	});
	return new Map(
		automationsQuery.data?.map((automation) => [
			automation.id,
			automation.name,
		]),
	);
};
