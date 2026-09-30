import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type { ChatAutomation } from "#/api/typesGenerated";

const chatAutomationsFamilyKey = ["chat-automations"] as const;

export const chatAutomationsKey = (organizationId: string) =>
	[...chatAutomationsFamilyKey, organizationId] as const;

export const chatAutomations = (organizationId: string) => ({
	queryKey: chatAutomationsKey(organizationId),
	queryFn: (): Promise<ChatAutomation[]> =>
		API.experimental.getChatAutomations(organizationId),
});

/** Refetches automation names, for example after new automation input. */
export const invalidateChatAutomations = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatAutomationsFamilyKey });
