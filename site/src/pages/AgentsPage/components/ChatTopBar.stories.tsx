import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet, useLocation } from "react-router";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { ChatTopBar } from "./ChatTopBar";

// Probe element rendered at /agents to verify search params are preserved
// when the mobile back button navigates away from a chat.
const AgentsSearchProbe = () => {
	const location = useLocation();
	return <div data-testid="agents-search">{location.search}</div>;
};

const requestArchiveAgent = fn<(chatId: string) => void>();
const requestArchiveAndDeleteWorkspace =
	fn<(chatId: string, workspaceId: string) => void>();
const requestUnarchiveAgent = fn<(chatId: string) => void>();
const requestPinAgent = fn<(chatId: string) => void>();
const requestUnpinAgent = fn<(chatId: string) => void>();
const onOpenRenameDialog = fn<(chat: TypesGen.Chat) => void>();

const chatTopBarOutletContext = {
	chatErrorReasons: {},
	setChatErrorReason: () => {},
	clearChatErrorReason: () => {},
	requestArchiveAgent,
	requestArchiveAndDeleteWorkspace,
	requestUnarchiveAgent,
	requestPinAgent,
	requestUnpinAgent,
	isArchiving: false,
	archivingChatId: undefined,
	activeChatChildren: undefined,
	onOpenRenameDialog,
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: fn(),
	onExpandSidebar: () => {},
	onChatReady: () => {},
} satisfies AgentsPageOutletContext;

const defaultProps = {
	chat: MockChat,
	panel: {
		showSidebarPanel: false,
		onToggleSidebar: fn(),
	},
} satisfies React.ComponentProps<typeof ChatTopBar>;

const meta: Meta<typeof ChatTopBar> = {
	title: "pages/AgentsPage/ChatTopBar",
	component: ChatTopBar,
	beforeEach: () => {
		requestArchiveAgent.mockClear();
		requestArchiveAndDeleteWorkspace.mockClear();
		requestUnarchiveAgent.mockClear();
		requestPinAgent.mockClear();
		requestUnpinAgent.mockClear();
		onOpenRenameDialog.mockClear();
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canShareChat: false,
		});
	},
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: { path: "/agents/chat-1" },
			routing: [
				{
					path: "/",
					element: <Outlet context={chatTopBarOutletContext} />,
					children: [{ path: "agents/:agentId", useStoryElement: true }],
				},
			],
		}),
	},
	args: defaultProps,
};
export default meta;
type Story = StoryObj<typeof ChatTopBar>;

export const Default: Story = {};

export const SharedChat: Story = {
	args: {
		chat: {
			...MockChat,
			shared: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByLabelText("Shared chat")).toBeInTheDocument();
		expect(canvas.queryByText("Shared")).not.toBeInTheDocument();
	},
};

export const WithPanelOpen: Story = {
	args: {
		panel: {
			showSidebarPanel: true,
			onToggleSidebar: () => {},
		},
	},
};

export const WithParentChat: Story = {
	args: {
		parentChat: {
			...MockChat,
			id: "parent-chat-1",
			title: "Set up CI/CD pipeline",
		},
	},
};

export const SidebarCollapsed: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/chat-1" },
			routing: [
				{
					path: "/",
					element: (
						<Outlet
							context={{
								...chatTopBarOutletContext,
								isSidebarCollapsed: true,
							}}
						/>
					),
					children: [{ path: "agents/:agentId", useStoryElement: true }],
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Expand sidebar" }),
		).toBeVisible();
	},
};

export const Archived: Story = {
	args: {
		chat: {
			...MockChat,
			archived: true,
		},
	},
};

export const NoTitle: Story = {
	args: {
		chat: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByLabelText("Open agent actions"),
		).not.toBeInTheDocument();
	},
};

export const WithOpenPR: Story = {
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/123",
				pull_request_title: "fix: resolve race condition in workspace builds",
				pull_request_draft: false,
				changes_requested: false,
				additions: 42,
				deletions: 7,
				changed_files: 5,
			},
		},
	},
};

export const WithDraftPR: Story = {
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/456",
				pull_request_title: "feat: add new notification system",
				pull_request_draft: true,
				changes_requested: false,
				additions: 120,
				deletions: 30,
				changed_files: 8,
			},
		},
	},
};

export const WithMergedPR: Story = {
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/789",
				pull_request_title: "chore: update dependencies",
				pull_request_state: "merged",
				pull_request_draft: false,
				changes_requested: false,
				additions: 5,
				deletions: 3,
				changed_files: 1,
			},
		},
	},
};

export const WithClosedPR: Story = {
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/101",
				pull_request_title: "fix: deprecated API cleanup",
				pull_request_state: "closed",
				pull_request_draft: false,
				changes_requested: false,
				additions: 0,
				deletions: 50,
				changed_files: 3,
			},
		},
	},
};

// ---------------------------------------------------------------
// Mobile viewport stories — constrain width to 390px so the
// responsive md: breakpoint triggers the compact PR number.
// ---------------------------------------------------------------

const mobileDecorator: Story["decorators"] = [
	(Story) => (
		<div style={{ width: 390 }}>
			<Story />
		</div>
	),
];

export const MobileWithOpenPR: Story = {
	decorators: mobileDecorator,
	parameters: { pixel: { matrix: { viewports: ["phone"] } } },
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/123",
				pull_request_title: "fix: resolve race condition in workspace builds",
				pull_request_draft: false,
				changes_requested: false,
				additions: 42,
				deletions: 7,
				changed_files: 5,
			},
		},
	},
};

export const MobileWithDraftPR: Story = {
	decorators: mobileDecorator,
	parameters: { pixel: { matrix: { viewports: ["phone"] } } },
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/456",
				pull_request_title: "feat: add new notification system",
				pull_request_draft: true,
				changes_requested: false,
				additions: 120,
				deletions: 30,
				changed_files: 8,
			},
		},
	},
};

export const MobileWithMergedPR: Story = {
	decorators: mobileDecorator,
	parameters: { pixel: { matrix: { viewports: ["phone"] } } },
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/789",
				pull_request_title: "chore: update dependencies",
				pull_request_state: "merged",
				pull_request_draft: false,
				changes_requested: false,
				additions: 5,
				deletions: 3,
				changed_files: 1,
			},
		},
	},
};

export const MobileWithClosedPR: Story = {
	decorators: mobileDecorator,
	parameters: { pixel: { matrix: { viewports: ["phone"] } } },
	args: {
		chat: {
			...MockChat,
			diff_status: {
				chat_id: "chat-1",
				url: "https://github.com/coder/coder/pull/101",
				pull_request_title: "fix: deprecated API cleanup",
				pull_request_state: "closed",
				pull_request_draft: false,
				changes_requested: false,
				additions: 0,
				deletions: 50,
				changed_files: 3,
			},
		},
	},
};

export const RenameChatItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Pin agent")).toBeInTheDocument();
			expect(
				body.getByRole("menuitem", { name: "Rename chat" }),
			).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Generate new title")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
		await userEvent.click(body.getByRole("menuitem", { name: "Rename chat" }));
		expect(onOpenRenameDialog).toHaveBeenCalledWith(MockChat);
	},
};

export const PinAgentItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Pin agent")).toBeInTheDocument();
			expect(
				body.getByRole("menuitem", { name: "Rename chat" }),
			).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
			expect(body.queryByText("Unpin agent")).not.toBeInTheDocument();
		});
		await userEvent.click(
			within(document.body).getByRole("menuitem", { name: "Pin agent" }),
		);
		expect(requestPinAgent).toHaveBeenCalledWith(MockChat.id);
	},
};

export const UnpinAgentItem: Story = {
	args: {
		chat: {
			...MockChat,
			pin_order: 1,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unpin agent")).toBeInTheDocument();
			expect(
				body.getByRole("menuitem", { name: "Rename chat" }),
			).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
			expect(body.queryByText("Pin agent")).not.toBeInTheDocument();
		});
		await userEvent.click(
			within(document.body).getByRole("menuitem", { name: "Unpin agent" }),
		);
		expect(requestUnpinAgent).toHaveBeenCalledWith(MockChat.id);
	},
};

export const ChildChatHidesPinAndArchiveActions: Story = {
	args: {
		chat: {
			...MockChat,
			parent_chat_id: "parent-chat-1",
			workspace_id: "workspace-1",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(
				body.getByRole("menuitem", { name: "Rename chat" }),
			).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Pin agent")).not.toBeInTheDocument();
		expect(body.queryByText("Unpin agent")).not.toBeInTheDocument();
		expect(body.queryByText("Archive agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
	},
};

export const ArchivedChildChatHasNoActionsMenu: Story = {
	args: {
		chat: {
			...MockChat,
			title: "Build authentication feature",
			parent_chat_id: "parent-chat-1",
			archived: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("Build authentication feature"),
			).toBeInTheDocument();
		});
		// Archive state is root-only, so an archived child chat has no menu
		// actions at all and the actions trigger is hidden entirely.
		expect(
			canvas.queryByLabelText("Open agent actions"),
		).not.toBeInTheDocument();
	},
};

export const ArchiveAndDeleteWorkspaceItem: Story = {
	args: {
		chat: {
			...MockChat,
			workspace_id: "workspace-1",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Archive agent")).toBeInTheDocument();
			expect(body.getByText("Archive & delete workspace")).toBeInTheDocument();
		});
		await userEvent.click(
			within(document.body).getByRole("menuitem", {
				name: "Archive & delete workspace",
			}),
		);
		expect(requestArchiveAndDeleteWorkspace).toHaveBeenCalledWith(
			MockChat.id,
			"workspace-1",
		);
	},
};

export const IdleChatArchiveActionsEnabled: Story = {
	args: {
		chat: {
			...MockChat,
			workspace_id: "workspace-1",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Open agent actions"));
		const body = within(document.body);
		const archiveItem = await body.findByRole("menuitem", {
			name: "Archive agent",
		});
		const archiveAndDeleteItem = body.getByRole("menuitem", {
			name: "Archive & delete workspace",
		});
		expect(archiveItem).not.toHaveAttribute("aria-disabled", "true");
		expect(archiveAndDeleteItem).not.toHaveAttribute("aria-disabled", "true");
		expect(
			body.queryByText("Interrupt or wait for the agent to finish first."),
		).not.toBeInTheDocument();
		await userEvent.click(archiveItem);
		expect(requestArchiveAgent).toHaveBeenCalledWith(MockChat.id);
	},
};

export const ActiveChatArchiveActionsDisabled: Story = {
	args: {
		chat: {
			...MockChat,
			workspace_id: "workspace-1",
		},
		liveChatStatus: "running",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Open agent actions"));
		const body = within(document.body);
		const archiveItem = await body.findByRole("menuitem", {
			name: "Archive agent",
		});
		const archiveAndDeleteItem = body.getByRole("menuitem", {
			name: "Archive & delete workspace",
		});
		expect(archiveItem).toHaveAttribute("aria-disabled", "true");
		expect(archiveAndDeleteItem).toHaveAttribute("aria-disabled", "true");
		const hint = "Interrupt or wait for the agent to finish first.";
		// The menu content fades in, so visibility needs a retry window.
		await waitFor(() => {
			expect(body.getByText(hint)).toBeVisible();
		});
		expect(archiveItem).toHaveAccessibleDescription(hint);
		expect(archiveAndDeleteItem).toHaveAccessibleDescription(hint);
		expect(requestArchiveAgent).not.toHaveBeenCalled();
	},
};

export const PreservesArchivedFilterOnMobileBack: Story = {
	decorators: mobileDecorator,
	parameters: {
		pixel: { matrix: { viewports: ["phone"] } },
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/chat-123",
				searchParams: { archived: "archived" },
			},
			routing: [
				{ path: "/agents/:agentId", useStoryElement: true },
				{ path: "/agents", element: <AgentsSearchProbe /> },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const backLink = await canvas.findByLabelText("Back");
		await userEvent.click(backLink);
		await waitFor(() => {
			expect(canvas.getByTestId("agents-search")).toHaveTextContent(
				"?archived=archived",
			);
		});
	},
};

export const ShareChatButton: Story = {
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		chat: {
			...MockChat,
			organization_id: MockDefaultOrganization.id,
		},
	},
	parameters: {
		user: MockUserOwner,
		queries: [
			{
				key: getAuthorizationKey({
					checks: {
						canShareChat: {
							object: {
								resource_type: "chat",
								owner_id: MockChat.owner_id,
								organization_id: MockDefaultOrganization.id,
							},
							action: "share",
						},
					},
				}),
				data: { canShareChat: true },
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canShareChat: true,
		});
		spyOn(API.experimental, "getChatACL").mockResolvedValue({
			users: [],
			groups: [],
		});
		spyOn(API.experimental, "updateChatACL").mockResolvedValue(undefined);
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember, MockOrganizationMember2],
			count: 2,
		});
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("Share")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Share" }),
		).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Share chat" }));
		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Chat sharing")).toBeVisible();
		});

		await userEvent.click(canvas.getByLabelText("Open agent actions"));
		await body.findByText("Rename chat");
		expect(
			body.queryByRole("menuitem", { name: "Share" }),
		).not.toBeInTheDocument();
	},
};

export const ShareChatButtonHiddenWithoutPermission: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("button", { name: "Share chat" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Share" }),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByLabelText("Open agent actions"));
		const body = within(document.body);
		await body.findByText("Rename chat");
		expect(
			body.queryByRole("menuitem", { name: "Share" }),
		).not.toBeInTheDocument();
	},
};

export const ArchivedWithUnarchive: Story = {
	args: {
		chat: {
			...MockChat,
			archived: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(
				body.getByRole("menuitem", { name: "Unarchive agent" }),
			).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Rename chat")).not.toBeInTheDocument();
		expect(body.queryByText("Pin agent")).not.toBeInTheDocument();
		expect(body.queryByText("Archive agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
		await userEvent.click(
			body.getByRole("menuitem", { name: "Unarchive agent" }),
		);
		expect(requestUnarchiveAgent).toHaveBeenCalledWith(MockChat.id);
	},
};
