import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChat, MockChatMessage } from "#/testHelpers/chatEntities";
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
import AgentChatPage from "./AgentChatPage";

const mockProjectChat = { ...MockChat, project_id: "project-1" };

const outletContext: Partial<AgentsPageOutletContext> = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	onChatReady: vi.fn(),
};

afterEach(() => {
	vi.restoreAllMocks();
});

const mockChatPageAPI = (messages: ChatMessage[] = []) => {
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${mockProjectChat.id}/stream`,
				websocketInit: (url, protocol) => createMockWebSocket(url, protocol)[0],
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(mockProjectChat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages,
		queued_messages: [],
		has_more: false,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				id: mockProjectChat.last_model_config_id,
				organization_id: mockProjectChat.organization_id,
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

const renderChatPage = () =>
	renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${mockProjectChat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

describe("AgentChatPage", () => {
	it("refreshes the project actions after a send fails", async () => {
		mockChatPageAPI();
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "deploy", action: true }]);
		const createChatMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockRejectedValue(
				mockApiError({ message: 'Skill "deploy" is not available.' }),
			);
		const user = userEvent.setup();

		const { queryClient } = renderChatPage();

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

	it("keeps the call line when an async answer that starts with a skill chip is edited", async () => {
		mockChatPageAPI([
			{
				...MockChatMessage,
				id: 5,
				chat_id: mockProjectChat.id,
				content: [
					{ type: "text", text: "Answer to ask_user_question call question:" },
					{
						type: "skill-invocation",
						skill_source: "personal",
						skill_name: "triage",
					},
					{ type: "text", text: "Backend" },
				],
			},
		]);
		const editChatMessage = vi
			.spyOn(API.experimental, "editChatMessage")
			.mockResolvedValue({ message: { ...MockChatMessage, id: 5 } });
		const user = userEvent.setup();

		renderChatPage();
		await user.click(
			await screen.findByRole("button", { name: "Edit message" }),
		);
		await user.click(screen.getByRole("button", { name: "Save Edit" }));

		await waitFor(() => expect(editChatMessage).toHaveBeenCalledTimes(1));
		expect(editChatMessage.mock.calls[0][2].content.slice(0, 2)).toEqual([
			{ type: "text", text: "Answer to ask_user_question call question:\n" },
			{
				type: "skill-invocation",
				skill_source: "personal",
				skill_name: "triage",
			},
		]);
	});
});
