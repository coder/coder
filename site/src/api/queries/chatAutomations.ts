import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type { ChatAutomation } from "#/api/typesGenerated";

/** Identifies an organization's automation list in the query cache. */
export const chatAutomationsKey = (organizationId: string) =>
	["chat-automations", organizationId] as const;

/** Lists the automations the caller can read in an organization. */
export const chatAutomations = (organizationId: string) =>
	queryOptions({
		queryKey: chatAutomationsKey(organizationId),
		queryFn: ({ signal }) =>
			API.experimental.getChatAutomations(organizationId, signal),
	});

/** Invalidates the owning organization's list after either deletion outcome. */
export const deleteChatAutomation = (queryClient: QueryClient) =>
	mutationOptions({
		mutationFn: (automation: ChatAutomation) =>
			API.experimental.deleteChatAutomation(
				automation.organization_id,
				automation.id,
			),
		onSettled: (_data, _error, automation) =>
			queryClient.invalidateQueries({
				queryKey: chatAutomationsKey(automation.organization_id),
			}),
	});
