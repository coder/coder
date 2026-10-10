import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { toast } from "sonner";
import {
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { API } from "#/api/api";
import { skillsKey } from "#/api/queries/skills";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockUserChatCompactionThresholds,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentChatPage from "./AgentChatPage";
import type { AgentsPageOutletContext } from "./AgentsPageLayout";

const mockChat: TypesGen.Chat = {
	...MockChat,
	id: "chat-1",
	status: "waiting",
	workspace_id: undefined,
	last_model_config_id: MockChatModel.id,
};

const outletContext: AgentsPageOutletContext = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	navigateAfterArchive: vi.fn(),
	activeChatChildren: undefined,
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: vi.fn(),
	onExpandSidebar: vi.fn(),
	onChatReady: vi.fn(),
};

const renderChatPage = () =>
	renderWithAuth(<Outlet context={outletContext} />, {
		route: `/agents/${mockChat.id}`,
		path: "/agents",
		children: [{ path: ":agentId", element: <AgentChatPage /> }],
	});

const mockChatPageRequests = () => {
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API, "getAIProviders").mockResolvedValue([]);
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(mockChat);
	vi.spyOn(API.experimental, "getChatMessages").mockResolvedValue({
		messages: [],
		queued_messages: [],
		has_more: false,
	});
	vi.spyOn(API.experimental, "getChatPrompts").mockResolvedValue({
		prompts: [],
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [
			{
				...MockChatModel,
				organization_id: mockChat.organization_id,
				is_default: true,
			},
		],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([]);
	vi.spyOn(API.experimental, "getUserChatDebugLogging").mockResolvedValue({
		debug_logging_enabled: false,
		user_toggle_allowed: false,
		forced_by_deployment: false,
	});
	vi.spyOn(
		API.experimental,
		"getUserChatCompactionThresholds",
	).mockResolvedValue(MockUserChatCompactionThresholds);
	return {
		clearChat: vi
			.spyOn(API.experimental, "clearChat")
			.mockResolvedValue(mockChat),
		compactChat: vi
			.spyOn(API.experimental, "compactChat")
			.mockResolvedValue(mockChat),
		createChatMessage: vi.spyOn(API.experimental, "createChatMessage"),
	};
};

const submitInComposer = async (text: string) => {
	const user = userEvent.setup();
	await user.click(await screen.findByTestId("chat-message-input"));
	await user.paste(text);
	// The first Enter accepts the highlighted menu entry, the second submits.
	await user.keyboard("{Enter}");
	await user.keyboard("{Enter}");
};

// Lexical reads selection geometry when text is pasted; jsdom has none.
beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

// Held commands stay in the composer, which saves a per-chat draft on unmount.
beforeEach(() => {
	localStorage.clear();
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AgentChatPage slash commands", () => {
	it("runs /clear when a personal skills refetch fails after a load", async () => {
		const { clearChat } = mockChatPageRequests();
		const getUserSkills = vi
			.spyOn(API.experimental, "getUserSkills")
			.mockResolvedValueOnce([])
			.mockRejectedValue(new Error("Failed to load skills."));

		const { queryClient } = renderChatPage();
		await waitFor(() => expect(getUserSkills).toHaveBeenCalledTimes(1));
		await queryClient.refetchQueries({
			queryKey: skillsKey({ type: "user", user: "me" }),
		});
		await submitInComposer("/clear");

		await waitFor(() => expect(clearChat).toHaveBeenCalledWith(mockChat.id));
	});

	it.each(["clear", "compact"])(
		"holds /%s while the organization skills request fails",
		async (command) => {
			const { clearChat, compactChat, createChatMessage } =
				mockChatPageRequests();
			vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
			vi.spyOn(API.experimental, "getOrganizationSkills").mockRejectedValue(
				new Error("Failed to load skills."),
			);
			const toastInfo = vi.spyOn(toast, "info");

			const { queryClient } = renderChatPage();
			await waitFor(() =>
				expect(
					queryClient.getQueryState(
						skillsKey({
							type: "organization",
							organizationId: mockChat.organization_id,
						}),
					)?.status,
				).toBe("error"),
			);
			await submitInComposer(`/${command}`);

			await waitFor(() =>
				expect(toastInfo).toHaveBeenCalledWith(
					`Checking whether /${command} is available. Try again in a moment.`,
				),
			);
			expect(clearChat).not.toHaveBeenCalled();
			expect(compactChat).not.toHaveBeenCalled();
			expect(createChatMessage).not.toHaveBeenCalled();
		},
	);
});
