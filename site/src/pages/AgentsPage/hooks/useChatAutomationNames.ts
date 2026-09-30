import { useQuery } from "react-query";
import { chatAutomations } from "#/api/queries/chatAutomations";
import { useDashboard } from "#/modules/dashboard/useDashboard";

/**
 * Maps automation IDs to names so automation input can be labeled.
 * The list is fetched only for chats that hold automation input. An
 * automation that was deleted or that the viewer cannot read is absent
 * from the map, and its label falls back to the automation ID. The chat
 * store invalidates the list when new automation input arrives, so live
 * labels pick up automations created or renamed after the page loaded.
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
