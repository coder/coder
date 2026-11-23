import { mutationOptions, type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	ChatAutomation,
	CreateChatAutomationRequest,
} from "#/api/typesGenerated";

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

/** Creates an automation without retaining its one-time secret after unmount. */
export const createChatAutomation = (queryClient: QueryClient) =>
	mutationOptions({
		gcTime: 0,
		retry: false,
		mutationFn: ({
			organizationId,
			req,
		}: {
			organizationId: string;
			req: CreateChatAutomationRequest;
		}) => API.experimental.createChatAutomation(organizationId, req),
		onSuccess: (_data, { organizationId }) =>
			queryClient.invalidateQueries({
				queryKey: chatAutomationsKey(organizationId),
			}),
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
