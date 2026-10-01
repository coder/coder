import type { QueryClient, UseQueryOptions } from "react-query";
import { API } from "#/api/api";
import type { ChatAutomation } from "#/api/typesGenerated";

const chatAutomationsFamilyKey = ["chat-automations"] as const;

export const chatAutomationsKey = (organizationId: string) =>
	[...chatAutomationsFamilyKey, organizationId] as const;

const chatAutomations = (organizationId: string) => ({
	queryKey: chatAutomationsKey(organizationId),
	queryFn: (): Promise<ChatAutomation[]> =>
		API.experimental.getChatAutomations(organizationId),
});

/** Automation IDs mapped to names. Deleted or unreadable automations are absent. */
export type ChatAutomationNameMap = ReadonlyMap<string, string>;

const selectChatAutomationNames = (
	automations: ChatAutomation[],
): ChatAutomationNameMap =>
	new Map(automations.map((automation) => [automation.id, automation.name]));

export const chatAutomationNameMap = (
	organizationId: string | undefined,
	{ enabled }: { enabled: boolean },
) =>
	({
		...chatAutomations(organizationId ?? ""),
		select: selectChatAutomationNames,
		enabled: Boolean(organizationId) && enabled,
	}) satisfies UseQueryOptions<
		ChatAutomation[],
		unknown,
		ChatAutomationNameMap
	>;

/** Refetches automation names, for example after new automation input. */
export const invalidateChatAutomations = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatAutomationsFamilyKey });
