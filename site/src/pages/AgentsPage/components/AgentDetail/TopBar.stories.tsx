import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { AgentDetailTopBar } from "./TopBar";

const defaultProps = {
	chatTitle: "Build authentication feature",
	panel: {
		showSidebarPanel: false,
		onToggleSidebar: () => {},
	},
	workspace: {
		canOpenEditors: true,
		canOpenWorkspace: true,
		onOpenInEditor: () => {},
		onViewWorkspace: () => {},
		onOpenTerminal: () => {},
		sshCommand: "ssh main.my-workspace.admin.coder",
	},
	onArchiveAgent: () => {},
	onArchiveAndDeleteWorkspace: () => {},
	onUnarchiveAgent: () => {},
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: () => {},
} satisfies React.ComponentProps<typeof AgentDetailTopBar>;

const meta: Meta<typeof AgentDetailTopBar> = {
	title: "pages/AgentsPage/AgentDetail/TopBar",
	component: AgentDetailTopBar,
	parameters: {
		layout: "fullscreen",
	},
	args: defaultProps,
};
export default meta;
type Story = StoryObj<typeof AgentDetailTopBar>;

export const Default: Story = {};

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
			owner_id: "owner-id",
			last_model_config_id: "model-config-1",
			mcp_server_ids: [],
			title: "Set up CI/CD pipeline",
			status: "completed",
			last_error: null,
			created_at: "2026-02-18T00:00:00.000Z",
			updated_at: "2026-02-18T00:00:00.000Z",
			archived: false,
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

export const ArchivedWithUnarchive: Story = {
	args: {
		isArchived: true,
		onUnarchiveAgent: () => {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Open the actions dropdown
		const trigger = canvas.getByLabelText("Open agent actions");
		await userEvent.click(trigger);
		// Verify "Unarchive Agent" is shown instead of "Archive Agent"
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unarchive Agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Archive Agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & Delete Workspace"),
		).not.toBeInTheDocument();
	},
};
