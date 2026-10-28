import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type { Chat } from "#/api/typesGenerated";
import { MockChat, MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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

const renderChatPage = (chat: Chat) =>
	renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${chat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

afterEach(() => {
	vi.restoreAllMocks();
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
		const user = userEvent.setup();
		const firstPage = renderChatPage(MockChat);

		await user.click(
			await screen.findByRole("button", {
				name: "Toggle overview",
				expanded: true,
			}),
		);
		expect(localStorage.getItem(overviewCardOpenStorageKey)).toBe("false");

		firstPage.unmount();
		renderChatPage(MockChat);
		await screen.findByRole("button", {
			name: "Toggle overview",
			expanded: false,
		});
	});

	it("hides the overview card while a row's panel is open", async () => {
		const chat = { ...MockChat, diff_statuses: [MockChatDiffStatus] };
		mockChatPageApi(chat);
		const user = userEvent.setup();
		renderChatPage(chat);

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
});
