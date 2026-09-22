import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	chat as chatQuery,
	organizationChatModelOverrides,
	userCompactionThresholds,
} from "#/api/queries/chats";
import type { Chat, ChatMessage, ChatModel } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatCompactionMessage,
	MockChatContextClean,
	MockChatContextDirty,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { MockChatModel } from "#/testHelpers/chatModels";
import { useChatContext } from "./useChatContext";

const dirtyChat: Chat = { ...MockChat, context: MockChatContextDirty };
const cleanChat: Chat = { ...MockChat, context: MockChatContextClean };
const chatModel: ChatModel = {
	...MockChatModel,
	id: MockChat.last_model_config_id,
	organization_id: MockChat.organization_id,
	context_limit: 200000,
};
const compactionModel: ChatModel = {
	...chatModel,
	id: "compaction",
	context_limit: 100000,
};
const messages: ChatMessage[] = [
	{
		...MockChatMessage,
		usage: {
			input_tokens: 100,
			cache_read_tokens: 20,
			cache_creation_tokens: 30,
			output_tokens: 900,
			reasoning_tokens: 700,
			context_limit: 200000,
		},
	},
];
type ContextState = ReturnType<typeof useChatContext>;

const Harness = ({
	chat = dirtyChat,
	chatMessages = messages,
	models = [chatModel, compactionModel],
	isReadOnly = false,
	onInspect,
}: {
	chat?: Chat;
	chatMessages?: readonly ChatMessage[];
	models?: readonly ChatModel[];
	isReadOnly?: boolean;
	onInspect: (state: ContextState) => void;
}) => {
	const state = useChatContext({
		chat,
		messages: chatMessages,
		models,
		isReadOnly,
	});
	return (
		<>
			<button
				type="button"
				onClick={state.onApplyContext}
				disabled={state.isApplyingContext}
			>
				Apply latest context
			</button>
			<button type="button" onClick={() => onInspect(state)}>
				Inspect context
			</button>
		</>
	);
};

const queryClients: QueryClient[] = [];
const setup = (
	props: Omit<Parameters<typeof Harness>[0], "onInspect"> = {},
) => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
			mutations: { retry: false },
		},
	});
	queryClients.push(queryClient);
	queryClient.setQueryData(
		chatQuery(MockChat.id).queryKey,
		props.chat ?? dirtyChat,
	);
	queryClient.setQueryData(userCompactionThresholds().queryKey, {
		thresholds: [{ model_config_id: chatModel.id, threshold_percent: 60 }],
	});
	queryClient.setQueryData(
		organizationChatModelOverrides(MockChat.organization_id).queryKey,
		{
			overrides: [
				{ context: "compaction", model_config_id: compactionModel.id },
			],
		},
	);
	const onInspect = vi.fn<(state: ContextState) => void>();
	const rendered = render(
		<QueryClientProvider client={queryClient}>
			<Harness {...props} onInspect={onInspect} />
		</QueryClientProvider>,
	);
	const user = userEvent.setup();
	return {
		...rendered,
		queryClient,
		onInspect,
		user,
		inspect: () =>
			user.click(screen.getByRole("button", { name: "Inspect context" })),
		apply: () =>
			user.click(screen.getByRole("button", { name: "Apply latest context" })),
	};
};

beforeEach(() => {
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(dirtyChat);
	vi.spyOn(
		API.experimental,
		"getUserChatCompactionThresholds",
	).mockResolvedValue({ thresholds: [] });
	vi.spyOn(
		API.experimental,
		"getOrganizationChatModelOverrides",
	).mockResolvedValue({ overrides: [] });
	vi.spyOn(API.experimental, "refreshChatContext").mockResolvedValue(cleanChat);
});

afterEach(() => {
	for (const queryClient of queryClients.splice(0)) queryClient.clear();
	vi.restoreAllMocks();
});

describe("useChatContext", () => {
	it("resolves context-only usage against the smaller compaction model and user threshold", async () => {
		const { inspect, onInspect } = setup();
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				contextUsage: expect.objectContaining({
					usedTokens: 150,
					contextLimitTokens: 100000,
					compressionThreshold: 60,
					context: dirtyChat.context,
				}),
			}),
		);
		expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
	});

	it.each([0, 100])(
		"retains known settings at %i%% before the first message or context snapshot",
		async (threshold) => {
			const { queryClient, inspect, onInspect } = setup({
				chat: MockChat,
				chatMessages: [],
			});
			await act(async () => {
				queryClient.setQueryData(userCompactionThresholds().queryKey, {
					thresholds: [
						{ model_config_id: chatModel.id, threshold_percent: threshold },
					],
				});
			});
			await inspect();
			expect(onInspect).toHaveBeenLastCalledWith(
				expect.objectContaining({
					contextUsage: {
						contextLimitTokens: 100000,
						compressionThreshold: threshold,
						context: undefined,
					},
					onApplyContext: undefined,
				}),
			);
			expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
		},
	);

	it("returns no usage when both context and settings are unknown", async () => {
		const { inspect, onInspect } = setup({
			chat: MockChat,
			chatMessages: [],
			models: [],
		});
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				contextUsage: null,
				onApplyContext: undefined,
			}),
		);
	});

	it("observes invalidated snapshots without applying context on mount or refetch", async () => {
		const { queryClient, inspect, onInspect } = setup();
		const updatedChat = {
			...dirtyChat,
			context: { ...MockChatContextDirty, error: "Scan failed" },
		};
		vi.mocked(API.experimental.getChat).mockResolvedValue(updatedChat);
		await act(async () => {
			await queryClient.invalidateQueries({
				queryKey: chatQuery(MockChat.id).queryKey,
				exact: true,
			});
		});
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				observedChat: updatedChat,
				contextUsage: expect.objectContaining({ context: updatedChat.context }),
			}),
		);
		expect(API.experimental.getChat).toHaveBeenCalledWith(MockChat.id);
		expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
	});

	it("only applies explicitly, shares the updated snapshot, and prevents another request while pending", async () => {
		let finish: (chat: Chat) => void = () => {};
		vi.mocked(API.experimental.refreshChatContext).mockImplementation(
			() =>
				new Promise<Chat>((resolve) => {
					finish = resolve;
				}),
		);
		const { apply, inspect, onInspect, queryClient } = setup();
		await apply();
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				isApplyingContext: true,
				applyContextSuccess: false,
			}),
		);
		await apply();
		expect(API.experimental.refreshChatContext).toHaveBeenCalledTimes(1);
		expect(API.experimental.refreshChatContext).toHaveBeenCalledWith(
			MockChat.id,
		);
		await act(async () => {
			finish(cleanChat);
		});
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				observedChat: cleanChat,
				contextUsage: expect.objectContaining({ context: cleanChat.context }),
				isApplyingContext: false,
				applyContextSuccess: true,
				onApplyContext: undefined,
			}),
		);
		expect(queryClient.getQueryData(chatQuery(MockChat.id).queryKey)).toEqual(
			cleanChat,
		);
	});

	it("preserves the pinned snapshot after failure and permits an explicit retry", async () => {
		const failure = new Error("Unable to apply context");
		vi.mocked(API.experimental.refreshChatContext).mockRejectedValueOnce(
			failure,
		);
		const { apply, inspect, onInspect, queryClient } = setup();
		await apply();
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				observedChat: dirtyChat,
				applyContextError: failure,
				applyContextSuccess: false,
				isApplyingContext: false,
			}),
		);
		expect(queryClient.getQueryData(chatQuery(MockChat.id).queryKey)).toEqual(
			dirtyChat,
		);
		await apply();
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				applyContextError: null,
				applyContextSuccess: true,
			}),
		);
		expect(API.experimental.refreshChatContext).toHaveBeenCalledTimes(2);
	});

	it.each([
		{ chat: { ...dirtyChat, archived: true } },
		{ isReadOnly: true },
		{ chat: cleanChat },
	])(
		"does not allow context application for restricted/current chat %j",
		async (props) => {
			const { apply, inspect, onInspect } = setup(props);
			await inspect();
			expect(onInspect).toHaveBeenLastCalledWith(
				expect.objectContaining({ onApplyContext: undefined }),
			);
			await apply();
			expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
		},
	);

	it("permits retrying a context error without claiming the snapshot is dirty", async () => {
		const { apply } = setup({
			chat: {
				...cleanChat,
				context: { ...MockChatContextClean, error: "Scan failed" },
			},
		});
		await apply();
		expect(API.experimental.refreshChatContext).toHaveBeenCalledWith(
			MockChat.id,
		);
	});

	it("retains the summary-only estimate marker with the effective window", async () => {
		const { inspect, onInspect } = setup({
			chatMessages: [MockChatCompactionMessage],
		});
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				contextUsage: expect.objectContaining({
					usedTokens: 12000,
					estimated: true,
					contextLimitTokens: 100000,
				}),
			}),
		);
	});

	it("does not invent an effective window or threshold when settings fail", async () => {
		vi.mocked(
			API.experimental.getUserChatCompactionThresholds,
		).mockRejectedValue(new Error("Forbidden"));
		vi.mocked(
			API.experimental.getOrganizationChatModelOverrides,
		).mockRejectedValue(new Error("Forbidden"));
		const { queryClient, inspect, onInspect } = setup();
		await act(async () => {
			await queryClient.resetQueries({
				queryKey: userCompactionThresholds().queryKey,
			});
			await queryClient.resetQueries({
				queryKey: organizationChatModelOverrides(MockChat.organization_id)
					.queryKey,
			});
		});
		await waitFor(() =>
			expect(
				queryClient.getQueryState(userCompactionThresholds().queryKey)?.status,
			).toBe("error"),
		);
		await inspect();
		expect(onInspect).toHaveBeenLastCalledWith(
			expect.objectContaining({
				contextUsage: expect.objectContaining({
					usedTokens: 150,
					contextLimitTokens: undefined,
					compressionThreshold: undefined,
				}),
			}),
		);
	});
});
