import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as apiModule from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockWorkspaceDebugChat,
	MockWorkspaceDebugChatMessages,
	MockWorkspaceDebugChatResponse,
} from "#/testHelpers/chatEntities";
import { MockFailedWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceDebugPanel } from "./WorkspaceDebugPanel";
import { getWorkspaceFailure } from "./workspaceFailure";

const { API } = apiModule;

type WatchChatSocket = ReturnType<typeof apiModule.watchChat>;

// The panel streams through the chat WebSocket; a silent socket keeps the
// store in its REST-hydrated state so the test can focus on requests.
const silentSocket = (): WatchChatSocket => ({
	url: "ws://example.test/api/v2/chats/stream",
	addEventListener: () => {},
	removeEventListener: () => {},
	close: () => {},
});

describe("WorkspaceDebugPanel", () => {
	it("opens the debugging chat for the failed build and sends follow-ups to it", async () => {
		const failure = getWorkspaceFailure(MockFailedWorkspace);
		expect(failure).toBeDefined();
		if (!failure) {
			return;
		}

		const createDebugChat = vi
			.spyOn(API.experimental, "createWorkspaceDebugChat")
			.mockResolvedValue(MockWorkspaceDebugChatResponse);
		vi.spyOn(API.experimental, "getChat").mockResolvedValue(
			MockWorkspaceDebugChat,
		);
		vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
			messages: [...MockWorkspaceDebugChatMessages].reverse(),
			queued_messages: [],
			has_more: false,
		});
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [],
			providers: [],
			unsupported_providers: [],
		});
		vi.spyOn(apiModule, "watchChat").mockImplementation(silentSocket);
		const sentMessage: TypesGen.ChatMessage = {
			id: 6,
			chat_id: MockWorkspaceDebugChat.id,
			created_at: "2024-01-01T00:00:01Z",
			role: "user",
			content: [{ type: "text", text: "Can I just retry the build?" }],
		};
		const sendMessage = vi
			.spyOn(API.experimental, "createChatMessage")
			.mockResolvedValue({ queued: false, message: sentMessage });

		render(
			<WorkspaceDebugPanel workspace={MockFailedWorkspace} failure={failure} />,
		);

		await waitFor(() => {
			expect(createDebugChat).toHaveBeenCalledWith(
				MockFailedWorkspace.latest_build.id,
			);
		});
		const input = await screen.findByRole("textbox", { name: "Message" });

		const user = userEvent.setup();
		await user.type(input, "Can I just retry the build?{enter}");

		await waitFor(() => {
			expect(sendMessage).toHaveBeenCalledWith(MockWorkspaceDebugChat.id, {
				content: [{ type: "text", text: "Can I just retry the build?" }],
			});
		});
		expect(input).toHaveValue("");
	});
});
