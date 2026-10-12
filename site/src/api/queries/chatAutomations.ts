import {
	infiniteQueryOptions,
	type QueryClient,
	queryOptions,
	type UseQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import { getErrorStatus } from "#/api/errors";
import { invalidateChatListQueries } from "#/api/queries/chats";
import type {
	Chat,
	ChatAutomation,
	ChatAutomationRunResponse,
	ChatAutomationSchedulePreviewRequest,
	CreateChatAutomationRequest,
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

export const webhookPublishEndpoint = (origin: string, automationId: string) =>
	`${origin}/api/experimental/chat-automations/${encodeURIComponent(automationId)}/events`;

/** Receives a new webhook secret, which never reaches the mutation cache. */
type OnWebhookSecret = (automationId: string, secret: string) => void;

export const createChatAutomation = (
	queryClient: QueryClient,
	organizationId: string,
	onWebhookSecret: OnWebhookSecret,
) => ({
	mutationFn: async (req: CreateChatAutomationRequest) => {
		const { webhook_secret, ...response } =
			await API.experimental.createChatAutomation(organizationId, req);
		if (webhook_secret) {
			onWebhookSecret(response.automation.id, webhook_secret);
		}
		return response;
	},
	onSettled: () =>
		queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		}),
});

export const rotateChatAutomationSecret = (
	queryClient: QueryClient,
	organizationId: string,
	onWebhookSecret: OnWebhookSecret,
) => ({
	mutationFn: async (automationId: string) => {
		const { webhook_secret, ...response } =
			await API.experimental.rotateChatAutomationSecret(
				organizationId,
				automationId,
			);
		onWebhookSecret(automationId, webhook_secret);
		return response;
	},
	onSettled: () =>
		queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		}),
});

export const chatAutomationSchedulePreviewKey = (
	organizationId: string,
	req: ChatAutomationSchedulePreviewRequest,
) =>
	[
		"chat-automation-schedule-preview",
		organizationId,
		req.schedule_cron,
		req.schedule_time_zone,
	] as const;

/** Lists the next runs of an unsaved schedule, as the server computes them. */
export const chatAutomationSchedulePreview = (
	organizationId: string,
	req: ChatAutomationSchedulePreviewRequest,
) =>
	queryOptions({
		queryKey: chatAutomationSchedulePreviewKey(organizationId, req),
		queryFn: ({ signal }) =>
			API.experimental.previewChatAutomationSchedule(
				organizationId,
				req,
				signal,
			),
		// A 400 is the server's answer about invalid input, not a transient error.
		retry: (failureCount, error) =>
			getErrorStatus(error) !== 400 && failureCount < 2,
	});

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

const automationChatsFamilyKey = ["chat-automation-chats"] as const;

export const automationChatsKey = (automationId: string) =>
	[...automationChatsFamilyKey, automationId] as const;

export const invalidateAutomationChats = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: automationChatsFamilyKey });

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
					// The history includes chats archived after the automation
					// created or wrote to them.
					q: "archived:any",
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
