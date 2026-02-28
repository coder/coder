import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatDiffStatusResponse } from "api/api";
import { AgentDetailTopBar } from "./TopBar";

const mockDiffStatus: ChatDiffStatusResponse = {
	chat_id: "chat-1",
	changes_requested: false,
	additions: 42,
	deletions: 7,
	changed_files: 5,
};

const defaultProps = {
	chatTitle: "Build authentication feature",
	onOpenParentChat: () => {},
	diff: {
		hasDiffStatus: false,
		diffStatus: undefined,
		showDiffPanel: false,
		onToggleFilesChanged: () => {},
	},
	workspace: {
		canOpenEditors: true,
		canOpenWorkspace: true,
		onOpenInEditor: () => {},
		onViewWorkspace: () => {},
	},
	onArchiveAgent: () => {},
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: () => {},
} satisfies React.ComponentProps<typeof AgentDetailTopBar>;

const meta: Meta<typeof AgentDetailTopBar> = {
	title: "pages/AgentsPage/AgentDetail/TopBar",
	component: AgentDetailTopBar,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
	args: defaultProps,
};
export default meta;
type Story = StoryObj<typeof AgentDetailTopBar>;

export const Default: Story = {};

export const WithDiffStats: Story = {
	args: {
		diff: {
			hasDiffStatus: true,
			diffStatus: mockDiffStatus,
			showDiffPanel: false,
			onToggleFilesChanged: () => {},
		},
	},
};

export const WithDiffPanelOpen: Story = {
	args: {
		diff: {
			hasDiffStatus: true,
			diffStatus: mockDiffStatus,
			showDiffPanel: true,
			onToggleFilesChanged: () => {},
		},
	},
};

export const WithParentChat: Story = {
	args: {
		parentChat: {
			id: "parent-chat-1",
			owner_id: "owner-id",
			last_model_config_id: "model-config-1",
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

export const NoTitle: Story = {
	args: {
		chatTitle: undefined,
	},
};
