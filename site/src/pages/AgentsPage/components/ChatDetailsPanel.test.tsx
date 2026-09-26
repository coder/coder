import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chat } from "#/api/queries/chats";
import {
	MockChat,
	MockChatContextClean,
	MockChatContextDirty,
} from "#/testHelpers/chatEntities";
import { ChatDetailsPanel } from "./ChatDetailsPanel";

vi.mock("#/modules/dashboard/useFeatureVisibility", () => ({
	useFeatureVisibility: () => ({ aibridge: true }),
}));

const props: ComponentProps<typeof ChatDetailsPanel> = {
	chatId: MockChat.id,
	isVisible: true,
	usage: {
		usedTokens: 12000,
		contextLimitTokens: 200000,
		compressionThreshold: 70,
		context: MockChatContextDirty,
	},
};
function setup(overrides: Partial<typeof props> = {}) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, gcTime: 0 } },
	});
	const wrapper = ({ children }: { children: ReactNode }) => (
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
			expect(API.experimental.getChatCost).toHaveBeenCalledWith("root-chat"),
		);
		expect(API.experimental.getChatCost).not.toHaveBeenCalledWith(MockChat.id);
	});
});
