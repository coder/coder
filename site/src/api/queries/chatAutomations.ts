import {
	infiniteQueryOptions,
	type QueryClient,
	type UseQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import { invalidateChatListQueries } from "#/api/queries/chats";
import type {
	Chat,
	ChatAutomation,
	ChatAutomationRunResponse,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";

const chatAutomationsFamilyKey = ["chat-automations"] as const;

export const chatAutomationsKey = (organizationId: string) =>
	[...chatAutomationsFamilyKey, organizationId] as const;

export const chatAutomations = (organizationId: string) => ({
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

export const updateChatAutomation = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: ({
		automationId,
		req,
	}: {
		automationId: string;
		req: UpdateChatAutomationRequest;
	}) =>
		API.experimental.updateChatAutomation(organizationId, automationId, req),
	onSettled: () =>
		queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		}),
});

const automationChatsKey = (automationId: string) =>
	["chat-automation-chats", automationId] as const;

const automationChatsPageSize = 25;

export const automationChats = (automationId: string) =>
	infiniteQueryOptions({
		queryKey: automationChatsKey(automationId),
		initialPageParam: 0,
		getNextPageParam: (lastPage: Chat[], pages: Chat[][]) =>
			lastPage.length < automationChatsPageSize
				? undefined
				: pages.length * automationChatsPageSize,
		queryFn: ({ pageParam, signal }) =>
			API.experimental.getChats(
				{
					automation_id: automationId,
					limit: automationChatsPageSize,
					offset: pageParam,
				},
				signal,
			),
	});

export const runChatAutomation = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (automationId: string): Promise<ChatAutomationRunResponse> =>
		API.experimental.runChatAutomation(organizationId, automationId),
	onSuccess: async (_: ChatAutomationRunResponse, automationId: string) => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: chatAutomationsKey(organizationId),
			}),
			queryClient.invalidateQueries({
				queryKey: automationChatsKey(automationId),
			}),
			invalidateChatListQueries(queryClient),
		]);
	},
});
