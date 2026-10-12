import {
	type InfiniteData,
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
	ChatAutomationLastRun,
	ChatAutomationReference,
	ChatAutomationRunResponse,
	ChatAutomationSchedulePreviewRequest,
	ChatAutomationsFilter,
	CreateChatAutomationRequest,
	UpdateChatAutomationRequest,
} from "#/api/typesGenerated";

const chatAutomationsFamilyKey = ["chat-automations"] as const;

/**
 * The organization's automation list. Without an owner the key is also the
 * prefix of every filtered list, so invalidating it refreshes them all.
 */
export const chatAutomationsKey = (
	organizationId: string,
	filter?: ChatAutomationsFilter,
) =>
	filter?.owner
		? ([
				...chatAutomationsFamilyKey,
				organizationId,
				{ owner: filter.owner },
			] as const)
		: ([...chatAutomationsFamilyKey, organizationId] as const);

export const chatAutomations = (
	organizationId: string,
	filter?: ChatAutomationsFilter,
) => ({
	queryKey: chatAutomationsKey(organizationId, filter),
	queryFn: (): Promise<ChatAutomation[]> =>
		API.experimental.getChatAutomations(organizationId, filter),
});

/** Automation IDs mapped to their references. Deleted automations are absent. */
export type ChatAutomationReferenceMap = ReadonlyMap<
	string,
	ChatAutomationReference
>;

const chatAutomationReferencesFamilyKey = [
	"chat-automation-references",
] as const;

export const chatAutomationReferencesKey = (chatId: string) =>
	[...chatAutomationReferencesFamilyKey, chatId] as const;

const selectChatAutomationReferences = (
	references: ChatAutomationReference[],
): ChatAutomationReferenceMap =>
	new Map(references.map((reference) => [reference.id, reference] as const));

/** Names the automations that delivered into a chat. */
export const chatAutomationReferences = (
	chatId: string,
	{ enabled }: { enabled: boolean },
) =>
	({
		queryKey: chatAutomationReferencesKey(chatId),
		queryFn: () => API.experimental.getChatAutomationReferences(chatId),
		select: selectChatAutomationReferences,
		enabled,
	}) satisfies UseQueryOptions<
		ChatAutomationReference[],
		unknown,
		ChatAutomationReferenceMap,
		ReturnType<typeof chatAutomationReferencesKey>
	>;

/** Omit chatId to refetch every chat. */
export const invalidateChatAutomationReferences = (
	queryClient: QueryClient,
	chatId?: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatId
			? chatAutomationReferencesKey(chatId)
			: chatAutomationReferencesFamilyKey,
	});

export const invalidateChatAutomations = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: chatAutomationsFamilyKey });

const isLoopbackHostname = (hostname: string) =>
	hostname === "localhost" ||
	hostname.endsWith(".localhost") ||
	hostname === "[::1]" ||
	/^127(\.\d{1,3}){3}$/.test(hostname);

/**
 * Returns the origin webhook senders must use. Senders must reach the
 * configured access URL, not the address this tab uses, unless the access
 * URL is a loopback address while the page is served from elsewhere, for
 * example through a port share: no outside sender can reach loopback.
 */
export const webhookOrigin = (accessURL: string, pageOrigin: string) => {
	const access = new URL(accessURL);
	const page = new URL(pageOrigin);
	if (
		isLoopbackHostname(access.hostname) &&
		!isLoopbackHostname(page.hostname)
	) {
		return page.origin;
	}
	return access.origin;
};

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
	// Not awaited, so the mutation stops pending when the response arrives
	// instead of after the list refetch.
	onSettled: () => {
		void queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		});
	},
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
	// Not awaited, so the mutation stops pending when the response arrives.
	onSettled: () => {
		void queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		});
	},
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
	// Awaited, unlike create and rotate: the list's enabled switch stays
	// pending until the refetched list shows the new state.
	onSettled: () =>
		queryClient.invalidateQueries({
			queryKey: chatAutomationsKey(organizationId),
		}),
});

const removeCachedAutomation = (
	queryClient: QueryClient,
	organizationId: string,
	automationId: string,
) =>
	// Every list of the organization, filtered or not.
	queryClient.setQueriesData<ChatAutomation[]>(
		{ queryKey: chatAutomationsKey(organizationId) },
		(automations) =>
			automations?.filter((automation) => automation.id !== automationId),
	);

export const deleteChatAutomation = (
	queryClient: QueryClient,
	organizationId: string,
) => ({
	mutationFn: (automationId: string) =>
		API.experimental.deleteChatAutomation(organizationId, automationId),
	// Drops the row right away, before the dialog closes, so focus moves to
	// a stable element instead of the removed row.
	onSuccess: (_: unknown, automationId: string) =>
		removeCachedAutomation(queryClient, organizationId, automationId),
	onError: (error: unknown, automationId: string) => {
		if (getErrorStatus(error) === 404) {
			removeCachedAutomation(queryClient, organizationId, automationId);
		}
	},
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

// The first page has no cursor.
const firstChatsPage: string | undefined = undefined;

// A chat that moves up between page requests can appear on two pages.
const selectUniqueChats = (data: InfiniteData<Chat[]>): Chat[] => {
	const seen = new Set<string>();
	return data.pages.flat().filter((chat) => {
		if (seen.has(chat.id)) {
			return false;
		}
		seen.add(chat.id);
		return true;
	});
};

export const automationChats = (automationId: string) =>
	infiniteQueryOptions({
		queryKey: automationChatsKey(automationId),
		initialPageParam: firstChatsPage,
		// Pages by cursor, not offset: new runs add chats to the top while the
		// dialog is open, which would shift every offset.
		getNextPageParam: (lastPage: Chat[]) =>
			lastPage.length < automationChatsPageSize
				? undefined
				: lastPage.at(-1)?.id,
		queryFn: ({ pageParam, signal }) =>
			API.experimental.getChats(
				{
					automation_id: automationId,
					// The history includes chats archived after the automation
					// created or wrote to them.
					q: "archived:any",
					limit: automationChatsPageSize,
					after_id: pageParam,
				},
				signal,
			),
		select: selectUniqueChats,
	});

/**
 * Reads, after a Run now failed, whether the server recorded the attempt as
 * a skip, such as a busy chat. A skip is the configured behavior, not an
 * error. Returns the new last run when it is a skip.
 */
export const fetchSkippedRun = async (
	queryClient: QueryClient,
	organizationId: string,
	automation: ChatAutomation,
): Promise<ChatAutomationLastRun | undefined> => {
	const automations = await queryClient.fetchQuery({
		...chatAutomations(organizationId),
		staleTime: 0,
	});
	const lastRun = automations.find(({ id }) => id === automation.id)?.last_run;
	const isNewRun = lastRun && lastRun.at !== automation.last_run?.at;
	return isNewRun && lastRun.outcome === "skipped" ? lastRun : undefined;
};

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
