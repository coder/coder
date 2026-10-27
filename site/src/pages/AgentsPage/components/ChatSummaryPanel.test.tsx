import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockChat,
	MockChatAttachment,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatSummaryPanel } from "./ChatSummaryPanel";

const mockOldestLoadedMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 41,
};

const mockNewestLoadedMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 42,
	role: "assistant",
};

const renderPanel = ({
	isVisible = true,
	hasMoreMessages = true,
}: {
	isVisible?: boolean;
	hasMoreMessages?: boolean;
} = {}) => {
	const store = createChatStore();
	store.upsertDurableMessages(
		[mockOldestLoadedMessage, mockNewestLoadedMessage],
		1,
	);
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: [],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<ChatSummaryPanel
					chatId={MockChat.id}
					isVisible={isVisible}
					store={store}
					hasMoreMessages={hasMoreMessages}
				/>
			</DashboardContext.Provider>
		</QueryClientProvider>,
	);
};

describe("ChatSummaryPanel attachments", () => {
	beforeEach(() => {
		vi.spyOn(API.experimental, "getChat").mockResolvedValue(MockChat);
	});

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
			name: "skips the request while hidden",
			isVisible: false,
			hasMoreMessages: true,
			calls: [],
		},
	])("$name", ({ isVisible, hasMoreMessages, calls }) => {
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockResolvedValue({ attachments: [MockChatAttachment] });

		renderPanel({ isVisible, hasMoreMessages });

		expect(getChatAttachments.mock.calls).toEqual(calls);
	});

	it("retries the earlier attachments after a failure", async () => {
		const user = userEvent.setup();
		const getChatAttachments = vi
			.spyOn(API.experimental, "getChatAttachments")
			.mockRejectedValueOnce(new Error("attachments unavailable"))
			.mockResolvedValueOnce({ attachments: [MockChatAttachment] });

		renderPanel();
		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getChatAttachments).toHaveBeenCalledTimes(2));
		expect(getChatAttachments.mock.calls[1]).toEqual([
			MockChat.id,
			{ before_id: mockOldestLoadedMessage.id },
			expect.any(AbortSignal),
		]);
	});
});
