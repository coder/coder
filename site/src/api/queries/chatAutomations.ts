import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type {
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

/** Refetches automation names, for example after new automation input. */
export const invalidateChatAutomations = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatAutomationsFamilyKey });

/**
 * Creates an automation. The response is never written to a query cache
 * because a webhook response carries the secret.
 */
export const createChatAutomation = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (req: CreateChatAutomationRequest) =>
		API.experimental.createChatAutomation(organizationId, req),
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
		retry: false,
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

const automationChatsKey = (automationId: string) =>
	["chat-automation-chats", automationId] as const;

export const automationChats = (automationId: string) =>
	queryOptions({
		queryKey: automationChatsKey(automationId),
		queryFn: ({ signal }) =>
			API.experimental.getChats(
				{ automation_id: automationId, limit: 25 },
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
		]);
	},
});
