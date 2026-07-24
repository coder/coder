import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { PopoverContent } from "#/components/Popover/Popover";
import { ChatTopBar } from "./ChatTopBar";

// Probe element rendered at /agents to verify search params are preserved
// when the mobile back button navigates away from a chat.
const AgentsSearchProbe = () => {
	const location = useLocation();
	return <div data-testid="agents-search">{location.search}</div>;
};

const defaultProps = {
	chatTitle: "Build authentication feature",
	panel: {
		showSidebarPanel: false,
		onToggleSidebar: fn(),
	},
	onArchiveAgent: fn(),
	onArchiveAndDeleteWorkspace: fn(),
	onPinAgent: fn(),
	onUnpinAgent: fn(),
	onOpenRenameDialog: fn(),
	onUnarchiveAgent: fn(),
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: fn(),
} satisfies React.ComponentProps<typeof ChatTopBar>;

const meta: Meta<typeof ChatTopBar> = {
	title: "pages/AgentsPage/ChatTopBar",
	component: ChatTopBar,
	parameters: {
		layout: "fullscreen",
	},
	args: defaultProps,
};
export default meta;
type Story = StoryObj<typeof ChatTopBar>;

export const Default: Story = {};

export const SharedChat: Story = {
	args: {
		isSharedChat: true,
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
			id: "parent-chat-1",
			organization_id: "test-org-id",
			owner_id: "owner-id",
			owner_username: "owner",
			last_model_config_id: "model-config-1",
			mcp_server_ids: [],
			labels: {},
			title: "Set up CI/CD pipeline",
			status: "waiting",
			last_turn_summary: null,
			summary: null,
			created_at: "2026-02-18T00:00:00.000Z",
			updated_at: "2026-02-18T00:00:00.000Z",
			archived: false,
			shared: false,
			pin_order: 0,
			has_unread: false,
			client_type: "ui",
			children: [],
		},
	},
};

export const SidebarCollapsed: Story = {
	args: {
		isSidebarCollapsed: true,
	},
};

export const Archived: Story = {
	args: {
		isArchived: true,
	},
};

export const NoTitle: Story = {
	args: {
		chatTitle: undefined,
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
		diffStatusData: {
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
};

export const WithDraftPR: Story = {
	args: {
		diffStatusData: {
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
};

export const WithMergedPR: Story = {
	args: {
		diffStatusData: {
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
};

export const WithClosedPR: Story = {
	args: {
		diffStatusData: {
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
	parameters: { chromatic: { viewports: [390] } },
	args: {
		diffStatusData: {
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
};

export const MobileWithDraftPR: Story = {
	decorators: mobileDecorator,
	parameters: { chromatic: { viewports: [390] } },
	args: {
		diffStatusData: {
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
};

export const MobileWithMergedPR: Story = {
	decorators: mobileDecorator,
	parameters: { chromatic: { viewports: [390] } },
	args: {
		diffStatusData: {
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
};

export const MobileWithClosedPR: Story = {
	decorators: mobileDecorator,
	parameters: { chromatic: { viewports: [390] } },
	args: {
		diffStatusData: {
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
};

export const RenameChatItem: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Pin agent")).toBeInTheDocument();
			expect(body.getByText("Rename chat")).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Generate new title")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
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
			expect(body.getByText("Rename chat")).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
			expect(body.queryByText("Unpin agent")).not.toBeInTheDocument();
		});
	},
};

export const UnpinAgentItem: Story = {
	args: {
		isPinned: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unpin agent")).toBeInTheDocument();
			expect(body.getByText("Rename chat")).toBeInTheDocument();
			expect(body.getByText("Archive agent")).toBeInTheDocument();
			expect(body.queryByText("Pin agent")).not.toBeInTheDocument();
		});
	},
};

export const ChildChatHidesPinAndArchiveActions: Story = {
	args: {
		isChildChat: true,
		hasWorkspace: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Rename chat")).toBeInTheDocument();
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
		isChildChat: true,
		isArchived: true,
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
		hasWorkspace: true,
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
	},
};

export const PreservesArchivedFilterOnMobileBack: Story = {
	decorators: mobileDecorator,
	parameters: {
		chromatic: { viewports: [390] },
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
	args: {
		renderChatSharingContent: () => (
			<PopoverContent align="end">Share chat</PopoverContent>
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("Share")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Share" }),
		).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Share chat" }));
		const body = within(document.body);
		expect(await body.findByText("Share chat")).toBeInTheDocument();

		await userEvent.click(canvas.getByLabelText("Open agent actions"));
		await body.findByText("Rename chat");
		expect(
			body.queryByRole("menuitem", { name: "Share" }),
		).not.toBeInTheDocument();
	},
};

export const ShareChatButtonHiddenWithoutPermission: Story = {
	args: {
		renderChatSharingContent: undefined,
	},
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
		isArchived: true,
		onUnarchiveAgent: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unarchive agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Rename chat")).not.toBeInTheDocument();
		expect(body.queryByText("Pin agent")).not.toBeInTheDocument();
		expect(body.queryByText("Archive agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
	},
};
