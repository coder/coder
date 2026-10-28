import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chat } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAttachment,
	MockChatContextClean,
	MockChatContextDirty,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatDetailsPanel } from "./ChatDetailsPanel";

vi.mock("#/modules/dashboard/useFeatureVisibility", () => ({
	useFeatureVisibility: () => ({ aibridge: true }),
}));

const props: React.ComponentProps<typeof ChatDetailsPanel> = {
	chatId: MockChat.id,
	isVisible: true,
	usage: {
		usedTokens: 12000,
		contextLimitTokens: 200000,
		compactionThreshold: {
			percent: 70,
			source: "model",
			organizationOverrideNotLoaded: false,
		},
		context: MockChatContextDirty,
	},
	store: createChatStore(),
	hasMoreMessages: false,
	isHydratingMessages: false,
};
function setup(overrides: Partial<typeof props> = {}) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, gcTime: 0 } },
	});
	const wrapper = ({ children }: { children: React.ReactNode }) => (
		<QueryClientProvider client={client}>{children}</QueryClientProvider>
	);
	const result = render(<ChatDetailsPanel {...props} {...overrides} />, {
		wrapper,
	});
	return {
		...result,
		client,
		rerenderPanel: (next: Partial<typeof props>) =>
			result.rerender(<ChatDetailsPanel {...props} {...overrides} {...next} />),
	};
}
beforeEach(() => {
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(MockChat);
	vi.spyOn(API.experimental, "getChatCost").mockResolvedValue({
		chat_id: MockChat.id,
		total_cost_micros: 1250000,
		request_count: 8,
		unpriced_request_count: 0,
	});
});

describe("ChatDetailsPanel", () => {
	it("keeps disclosure state independent across keyboard use and query updates, resetting for another chat", async () => {
		const user = userEvent.setup();
		const onApplyContext = vi.fn();
		const { client, rerenderPanel } = setup({ onApplyContext });
		await user.tab();
		expect(
			screen.getByRole("button", { name: "Summary", expanded: true }),
		).toHaveFocus();
		await user.keyboard("{Enter}");
		await user.tab();
		expect(
			screen.getByRole("button", { name: "Apply latest context" }),
		).toHaveFocus();
		await user.tab();
		await user.keyboard(" ");
		expect(
			screen.getByRole("button", { name: /^Context/, expanded: true }),
		).toHaveFocus();
		await user.tab();
		await user.keyboard("{Enter}");
		expect(
			screen.getByRole("button", { name: /^Skills/, expanded: true }),
		).toHaveFocus();
		await user.tab();
		await user.keyboard(" ");
		expect(
			screen.getByRole("button", { name: /^MCP servers/, expanded: true }),
		).toHaveFocus();
		act(() =>
			client.setQueryData(chat(MockChat.id).queryKey, {
				...MockChat,
				summary: "Updated summary",
			}),
		);
		await user.tab({ shift: true });
		expect(
			screen.getByRole("button", { name: /^Skills/, expanded: true }),
		).toHaveFocus();
		await user.tab({ shift: true });
		expect(
			screen.getByRole("button", { name: /^Context/, expanded: true }),
		).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(
			screen.getByRole("button", { name: /^Context/, expanded: false }),
		).toHaveFocus();
		expect(onApplyContext).not.toHaveBeenCalled();
		rerenderPanel({ chatId: "other-chat" });
		await user.tab();
		expect(
			screen.getByRole("button", { name: "Summary", expanded: true }),
		).toHaveFocus();
		await user.tab();
		await user.tab();
		expect(
			screen.getByRole("button", { name: /^Context/, expanded: false }),
		).toHaveFocus();
	});
	it("applies explicitly, prevents duplicates while pending, and hands off focus after success", async () => {
		const user = userEvent.setup();
		const onApplyContext = vi.fn();
		const { rerenderPanel } = setup({ onApplyContext });
		await user.click(
			screen.getByRole("button", { name: "Apply latest context" }),
		);
		expect(onApplyContext).toHaveBeenCalledTimes(1);
		rerenderPanel({ isApplyingContext: true });
		const pending = screen.getByRole("button", {
			name: "Applying latest context...",
		});
		expect(pending).toHaveFocus();
		await user.keyboard("{Enter}");
		await user.click(pending);
		expect(onApplyContext).toHaveBeenCalledTimes(1);
		rerenderPanel({
			isApplyingContext: false,
			applySuccess: true,
			usage: { ...props.usage, context: MockChatContextClean },
		});
		expect(
			screen.getByRole("group", { name: "Workspace context" }),
		).toHaveFocus();
	});
	it("keeps retry focused on failure and does not steal focus on later success", async () => {
		const user = userEvent.setup();
		const onApplyContext = vi.fn();
		const { rerenderPanel } = setup({ onApplyContext });
		await user.click(
			screen.getByRole("button", { name: "Apply latest context" }),
		);
		rerenderPanel({ applyError: new Error("Apply failed") });
		expect(
			screen.getByRole("button", { name: "Retry applying latest context" }),
		).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(onApplyContext).toHaveBeenCalledTimes(2);
		await user.tab();
		const context = screen.getByRole("button", { name: /^Context/ });
		rerenderPanel({
			applySuccess: true,
			usage: { ...props.usage, context: MockChatContextClean },
		});
		expect(context).toHaveFocus();
	});
	it("keeps a read-only dirty snapshot navigable without an Apply action", async () => {
		const user = userEvent.setup();
		setup();
		await user.tab();
		await user.tab();
		expect(screen.getByRole("button", { name: /^Context/ })).toHaveFocus();
	});
	it("does not fetch hidden Details and fetches the root tree cost when shown", async () => {
		vi.mocked(API.experimental.getChat).mockResolvedValue({
			...MockChat,
			parent_chat_id: "parent-chat",
			root_chat_id: "root-chat",
		});
		const { rerenderPanel } = setup({ isVisible: false });
		expect(API.experimental.getChat).not.toHaveBeenCalled();
		expect(API.experimental.getChatCost).not.toHaveBeenCalled();
		rerenderPanel({ isVisible: true });
		await waitFor(() =>
			expect(API.experimental.getChatCost).toHaveBeenCalledWith(
				"root-chat",
				expect.any(AbortSignal),
			),
		);
		expect(API.experimental.getChatCost).not.toHaveBeenCalledWith(
			MockChat.id,
			expect.any(AbortSignal),
		);
	});
});

describe("ChatDetailsPanel attachments", () => {
	const mockOldestLoadedMessage: TypesGen.ChatMessage = {
		...MockChatMessage,
		id: 41,
	};
	const mockNewestLoadedMessage: TypesGen.ChatMessage = {
		...MockChatMessage,
		id: 42,
		role: "assistant",
	};
	const setupWithMessages = (overrides: Partial<typeof props> = {}) => {
		const store = createChatStore();
		store.replaceMessages([mockOldestLoadedMessage, mockNewestLoadedMessage]);
		return setup({ store, hasMoreMessages: true, ...overrides });
	};

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it.each([
		{
			name: "asks for attachments before the loaded page",
			isVisible: true,
			hasMoreMessages: true,
			calls: [
				[
					MockChat.id,
					{ before_id: mockOldestLoadedMessage.id },
					expect.any(AbortSignal),
				],
			],
		},
		{
			name: "skips the request when all are loaded",
			isVisible: true,
			hasMoreMessages: false,
			calls: [],
		},
		{
			name: "skips the request during hydration",
			isVisible: true,
			hasMoreMessages: false,
			isHydratingMessages: true,
			calls: [],
		},
		{
			name: "skips the request while hidden",
			isVisible: false,
			hasMoreMessages: true,
			calls: [],
		},
	])("$name", ({ isVisible, hasMoreMessages, isHydratingMessages, calls }) => {
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockResolvedValue({ attachments: [MockChatAttachment] });

		setupWithMessages({ isVisible, hasMoreMessages, isHydratingMessages });

		expect(getChatAttachments.mock.calls).toEqual(calls);
	});

	it("retries the earlier attachments after a failure", async () => {
		const user = userEvent.setup();
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(new Error("attachments unavailable"))
			.mockResolvedValueOnce({ attachments: [MockChatAttachment] });

		setupWithMessages();
		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getChatAttachments).toHaveBeenCalledTimes(2));
		expect(getChatAttachments.mock.calls[1]).toEqual([
			MockChat.id,
			{ before_id: mockOldestLoadedMessage.id },
			expect.any(AbortSignal),
		]);
	});
});
