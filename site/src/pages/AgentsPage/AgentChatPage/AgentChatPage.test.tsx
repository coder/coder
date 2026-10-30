import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
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

const renderChatPage = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
) => {
	vi.spyOn(apiModule, "watchChat").mockImplementation(
		() =>
			new OneWayWebSocket({
				apiRoute: `/api/experimental/chats/${mockProjectChat.id}/stream`,
				websocketInit: (url, protocol) => createMockWebSocket(url, protocol)[0],
			}),
	);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(mockProjectChat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: queuedMessages,
		has_more: false,
		snapshot_version: 1,
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
	return renderWithAuth(<Outlet context={outletContext} />, {
		path: "/agents",
		route: `/agents/${mockProjectChat.id}`,
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});
};

describe("AgentChatPage", () => {
	it("refreshes the project actions after a send fails", async () => {
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

		// The actions bar remounts after the send, so only the failure's
		// invalidation may refetch the list.
		queryClient.setQueryDefaults(
			chatProjectSkillsKey(mockProjectChat.project_id),
			{ staleTime: Number.POSITIVE_INFINITY },
		);
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

	it("reports a failed promote and keeps the row queued", async () => {
		vi.spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue([]);
		const promote = vi
			.spyOn(API.experimental, "promoteChatQueuedMessage")
			.mockRejectedValue(mockApiError({ message: "promote failed" }));
		const user = userEvent.setup();

		renderChatPage([MockChatQueuedMessage]);

		await user.click(await screen.findByRole("button", { name: "Send now" }));
		await waitFor(() =>
			expect(outletContext.setChatErrorReason).toHaveBeenCalledWith(
				mockProjectChat.id,
				expect.objectContaining({ message: "promote failed" }),
			),
		);
		// The row is listed again, so it can be promoted again.
		await user.click(screen.getByRole("button", { name: "Send now" }));
		await waitFor(() => expect(promote).toHaveBeenCalledTimes(2));
	});
});
