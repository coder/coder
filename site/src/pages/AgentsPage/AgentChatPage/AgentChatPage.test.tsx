import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type {
	ChatMessage,
	CreateChatMessageResponse,
} from "#/api/typesGenerated";
import { MockChat, MockChatMessage } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { MockAsyncQuestionMessages } from "../components/ChatConversation/storyFixtures";
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
	vi.unstubAllGlobals();
});

const mockChatPageAPI = (messages: ChatMessage[] = []) => {
	vi.spyOn(API, "getExperiments").mockResolvedValue(["chat-projects"]);
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

	const answerFromDockOnNarrowScreen = async () => {
		const originalMatchMedia = window.matchMedia;
		vi.stubGlobal("matchMedia", (query: string) => ({
			...originalMatchMedia(query),
			matches: query === mobileViewportMediaQuery,
		}));
		const user = userEvent.setup();
		renderChatPage();
		const dock = await screen.findByRole("region", { name: "Open question" });
		const composer = screen.getByRole("textbox", { name: "Chat message" });
		// jsdom focuses the editor only while the DOM selection is in it, and a
		// click on the form would move the selection out, so answer by keyboard.
		await user.click(composer);
		act(() =>
			within(dock)
				.getByRole("radio", { name: /^Frontend/ })
				.focus(),
		);
		await user.keyboard(" ");
		act(() => within(dock).getByRole("button", { name: "Submit" }).focus());
		await user.keyboard("{Enter}");
		return { dock, composer };
	};

	it("focuses the chat input after an answer sends on a narrow screen", async () => {
		mockChatPageAPI(MockAsyncQuestionMessages);
		vi.spyOn(API.experimental, "createChatMessage").mockResolvedValue({
			queued: true,
		});

		const { composer } = await answerFromDockOnNarrowScreen();

		await waitFor(() => expect(composer).toHaveFocus());
	});

	it("leaves focus out of the chat input when an answer fails to send", async () => {
		mockChatPageAPI(MockAsyncQuestionMessages);
		vi.spyOn(API.experimental, "createChatMessage").mockRejectedValue(
			new Error("The answer failed."),
		);

		const { dock, composer } = await answerFromDockOnNarrowScreen();

		await within(dock).findByText("The answer failed.");
		expect(composer).not.toHaveFocus();
	});

	it("leaves focus where the user moved it while an answer was sending", async () => {
		mockChatPageAPI(MockAsyncQuestionMessages);
		const pendingSend = createDeferred<CreateChatMessageResponse>();
		vi.spyOn(API.experimental, "createChatMessage").mockReturnValue(
			pendingSend.promise,
		);

		const { composer } = await answerFromDockOnNarrowScreen();
		const editButton = screen.getByRole("button", { name: "Edit message" });
		act(() => editButton.focus());
		await act(async () => pendingSend.resolve({ queued: true }));

		expect(editButton).toHaveFocus();
		expect(composer).not.toHaveFocus();
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
