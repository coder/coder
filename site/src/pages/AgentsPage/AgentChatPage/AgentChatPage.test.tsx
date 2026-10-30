import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type { Chat } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatDiffStatus,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { MockSkill } from "#/testHelpers/skills";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { overviewCardOpenStorageKey } from "../utils/overviewCardStorage";
import AgentChatPage from "./AgentChatPage";

const mockProjectChat = { ...MockChat, project_id: "project-1" };

const outletContext: Partial<AgentsPageOutletContext> = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	onChatReady: vi.fn(),
};

const mockChatPageApi = (chat: Chat) => {
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${chat.id}/stream`,
				websocketInit: (url, protocol) => createMockWebSocket(url, protocol)[0],
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(chat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: [],
		has_more: false,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				id: chat.last_model_config_id,
				organization_id: chat.organization_id,
				is_default: true,
			},
		],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
};

/** The transcript has messages before the loaded page. */
const mockOlderMessages = () =>
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [MockChatMessage],
		queued_messages: [],
		has_more: true,
	});

const renderChatPage = (chat: Chat) =>
	renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${chat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

/**
 * Lets the test set the chat width that the overview card measures. Call it
 * before rendering the page.
 */
const stubChatWidth = () => {
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
	document.documentElement.style.fontSize = "16px";
	return async (widthPx: number) => {
		// The page re-renders its top bar while it loads, so look the observer
		// up again on every try. The last observer of the chat is the live one.
		const chatObserver = () =>
			MockResizeObserver.instances.findLast((observer) =>
				observer.observe.mock.calls.some(
					([element]) =>
						element instanceof HTMLElement &&
						element.contains(
							screen.queryByRole("button", { name: "Toggle panel" }),
						),
				),
			);
		await waitFor(() => expect(chatObserver()).toBeDefined());
		act(() => chatObserver()?.simulateResize(widthPx));
	};
};

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
	document.documentElement.style.fontSize = "";
	localStorage.clear();
});

describe("AgentChatPage", () => {
	it("refreshes the project actions after a send fails", async () => {
		mockChatPageApi(mockProjectChat);
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "deploy", action: true }]);
		const createChatMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockRejectedValue(
				mockApiError({ message: 'Skill "deploy" is not available.' }),
			);
		const user = userEvent.setup();

		const { queryClient } = renderChatPage(mockProjectChat);

		await user.click(await screen.findByRole("button", { name: "deploy" }));
		// The page and the actions bar share the list, so let both settle.
		await waitFor(() =>
			expect(
				queryClient.isFetching({
					queryKey: chatProjectSkillsKey(mockProjectChat.project_id),
				}),
			).toBe(0),
		);
		const listRequestsBeforeSend = getChatProjectSkills.mock.calls.length;
		await user.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(createChatMessage).toHaveBeenCalledTimes(1));
		await waitFor(() =>
			expect(getChatProjectSkills).toHaveBeenCalledTimes(
				listRequestsBeforeSend + 1,
			),
		);
	});

	it("shows the overview card in a new chat and keeps the toggle's choice", async () => {
		mockChatPageApi(MockChat);
		const setChatWidth = stubChatWidth();
		const user = userEvent.setup();
		const firstPage = renderChatPage(MockChat);
		await setChatWidth(1600);

		await user.click(
			await screen.findByRole("button", {
				name: "Toggle overview",
				expanded: true,
			}),
		);
		expect(localStorage.getItem(overviewCardOpenStorageKey)).toBe("false");

		firstPage.unmount();
		renderChatPage(MockChat);
		await setChatWidth(1600);
		await screen.findByRole("button", {
			name: "Toggle overview",
			expanded: false,
		});
	});

	it("hides the overview card while a row's panel is open", async () => {
		const chat = { ...MockChat, diff_statuses: [MockChatDiffStatus] };
		mockChatPageApi(chat);
		const setChatWidth = stubChatWidth();
		const user = userEvent.setup();
		renderChatPage(chat);
		await setChatWidth(1600);

		const card = await screen.findByRole("complementary", {
			name: "Overview",
		});
		await user.click(within(card).getByRole("button", { name: /#123/ }));
		expect(
			screen.queryByRole("complementary", { name: "Overview" }),
		).toBeNull();

		await user.click(screen.getByRole("button", { name: "Toggle panel" }));
		await screen.findByRole("button", {
			name: "Toggle overview",
			expanded: true,
		});
		expect(localStorage.getItem(overviewCardOpenStorageKey)).toBeNull();
	});

	it("opens a pull request row's diff in the Git tab and focuses the tab", async () => {
		const mockOtherRef = {
			...MockChatDiffStatus,
			git_branch: "feat/other",
			pr_number: 23020,
			url: "https://github.com/coder/coder/pull/23020",
		};
		const mockTwoPullRequestsChat = {
			...MockChat,
			diff_status: MockChatDiffStatus,
			diff_statuses: [mockOtherRef, MockChatDiffStatus],
		};
		mockChatPageApi(mockTwoPullRequestsChat);
		vi.spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			chat_id: mockTwoPullRequestsChat.id,
		});
		const setChatWidth = stubChatWidth();
		const user = userEvent.setup();
		renderChatPage(mockTwoPullRequestsChat);
		await setChatWidth(1600);

		const card = await screen.findByRole("complementary", {
			name: "Overview",
		});
		await user.click(within(card).getByRole("button", { name: /#23020/ }));

		expect(screen.getByRole("tab", { name: "Git" })).toHaveFocus();
		expect(screen.getByRole("link", { name: "View PR" })).toHaveAttribute(
			"href",
			mockOtherRef.url,
		);
	});

	it("does not mount the overview card or request its data in a narrow chat", async () => {
		mockChatPageApi(MockChat);
		mockOlderMessages();
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockResolvedValue({ attachments: [] });
		const setChatWidth = stubChatWidth();
		renderChatPage(MockChat);

		await setChatWidth(84 * 16 - 1);

		expect(
			screen.queryByRole("button", { name: "Toggle overview" }),
		).toBeNull();
		expect(
			screen.queryByRole("complementary", { name: "Overview" }),
		).toBeNull();
		expect(getChatAttachments).not.toHaveBeenCalled();
	});

	it("retries the overview card's earlier attachments after a failure", async () => {
		mockChatPageApi(MockChat);
		mockOlderMessages();
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(new Error("Failed to get chat attachments."))
			.mockResolvedValueOnce({ attachments: [] });
		const setChatWidth = stubChatWidth();
		const user = userEvent.setup();
		renderChatPage(MockChat);
		await setChatWidth(1600);

		const card = await screen.findByRole("complementary", {
			name: "Overview",
		});
		await user.click(
			await within(card).findByRole("button", { name: "Retry" }),
		);

		await waitFor(() => expect(getChatAttachments).toHaveBeenCalledTimes(2));
	});
});
