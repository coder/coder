import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Chat } from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ReviewUnreadDialog } from "./ReviewUnreadDialog";

const now = new Date().toISOString();
const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: `chat-${Math.random().toString(36).slice(2)}`,
	owner_id: "owner-1",
	title: "Untitled chat",
	status: "completed",
	last_model_config_id: "config-1",
	mcp_server_ids: [],
	labels: {},
	created_at: oneHourAgo,
	updated_at: now,
	archived: false,
	pin_order: 0,
	has_unread: true,
	last_error: null,
	...overrides,
});

const unreadChats = [
	buildChat({
		id: "chat-1",
		title: "Fix authentication bug in login flow",
		status: "completed",
	}),
	buildChat({
		id: "chat-2",
		title: "Add dark mode support to settings page",
		status: "waiting",
	}),
	buildChat({
		id: "chat-3",
		title: "Refactor database queries for performance",
		status: "running",
	}),
	buildChat({
		id: "chat-4",
		title: "Write unit tests for user service",
		status: "error",
		last_error: "Test runner process exited unexpectedly",
	}),
	buildChat({
		id: "chat-5",
		title: "Update API documentation for v2 endpoints",
		status: "completed",
	}),
];

const meta: Meta<typeof ReviewUnreadDialog> = {
	title: "pages/AgentsPage/ReviewUnreadDialog",
	component: ReviewUnreadDialog,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		open: true,
		onOpenChange: fn(),
		onChatReviewed: fn(),
		unreadChats,
	},
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
};

export default meta;
type Story = StoryObj<typeof ReviewUnreadDialog>;

export const Default: Story = {};

export const LastChat: Story = {
	args: {
		unreadChats: [unreadChats[0]],
	},
};

export const WithError: Story = {
	args: {
		unreadChats: [unreadChats[3], unreadChats[0]],
	},
};
