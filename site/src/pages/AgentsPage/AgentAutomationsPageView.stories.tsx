import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Chat, ChatAutomation } from "#/api/typesGenerated";
import { MockChat, MockChatAutomation } from "#/testHelpers/chatEntities";
import { MockUserOwner, mockApiError } from "#/testHelpers/entities";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";

const mockTargetChat: Chat = {
	...MockChat,
	id: "chat-target",
	title: "Nightly build triage",
};
const mockCreatingChat: Chat = {
	...MockChat,
	id: "chat-creator",
	title: "Set up CI monitoring",
};
const mockArchivedChat: Chat = {
	...MockChat,
	id: "chat-archived",
	title: "Old release thread",
	archived: true,
};

const mockScheduleAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-schedule",
	name: "Nightly build check",
	target_chat_id: mockTargetChat.id,
	target_chat: { id: mockTargetChat.id, title: mockTargetChat.title },
	schedule_cron: "0 9 * * 1-5",
	schedule_time_zone: "Europe/Berlin",
	// The first entry has passed, so the row shows the second one.
	next_run_times: ["2026-01-01T08:00:00Z", "2099-10-01T07:00:00Z"],
};
const mockHeartbeatAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-heartbeat",
	name: "CI heartbeat",
	created_by_chat_id: mockCreatingChat.id,
	created_by_chat: { id: mockCreatingChat.id, title: mockCreatingChat.title },
	target_chat_id: mockCreatingChat.id,
	target_chat: { id: mockCreatingChat.id, title: mockCreatingChat.title },
	schedule_cron: "*/5 * * * *",
	schedule_time_zone: "UTC",
	next_run_times: ["2099-09-30T12:05:00Z"],
};
const mockWebhookAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-webhook",
	name: "Deploy finished",
	kind: "webhook",
	enabled: false,
	target_mode: "new_chat",
	target_chat_id: undefined,
	when_busy: undefined,
	webhook_use: "single",
	webhook_consumed_at: "2026-09-29T10:00:00Z",
	schedule_cron: undefined,
	schedule_time_zone: undefined,
};
const mockDigestAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-digest",
	name: "Morning digest",
	target_mode: "new_chat",
	target_chat_id: undefined,
	when_busy: undefined,
	new_chat_model_config_id: "model-1",
	schedule_cron: "30 8 * * *",
	schedule_time_zone: "America/New_York",
	next_run_times: ["2099-10-01T12:30:00Z"],
};
const mockListeningWebhook: ChatAutomation = {
	...mockWebhookAutomation,
	id: "automation-listening",
	name: "PR opened",
	enabled: true,
	webhook_use: "multi",
	webhook_consumed_at: undefined,
};
const mockDisabledSchedule: ChatAutomation = {
	...mockScheduleAutomation,
	id: "automation-disabled",
	name: "Weekly report",
	enabled: false,
	schedule_cron: "0 9 * * 1",
};
const mockPausedSchedule: ChatAutomation = {
	...mockDigestAutomation,
	id: "automation-paused",
	name: "Release notes",
	paused_reasons: ["model_unavailable"],
	next_run_times: [],
};

const meta: Meta<typeof AgentAutomationsPageView> = {
	title: "pages/AgentsPage/AgentAutomationsPageView",
	component: AgentAutomationsPageView,
	args: {
		currentUserId: MockUserOwner.id,
		automations: [
			mockScheduleAutomation,
			mockHeartbeatAutomation,
			mockDigestAutomation,
			mockListeningWebhook,
			mockWebhookAutomation,
			mockDisabledSchedule,
			mockPausedSchedule,
		],
		isLoading: false,
		error: undefined,
		onDismissRunError: fn(),
		onToggleEnabled: fn(),
		onRunNow: fn(),
		onViewChats: fn(),
		onCreateAutomation: fn(),
		onEditAutomation: fn(),
		onDeleteAutomation: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentAutomationsPageView>;

export const WithAutomations: Story = {};

export const OtherOwner: Story = {
	args: {
		currentUserId: "another-user-id",
		automations: [
			{
				...mockScheduleAutomation,
				owner: { id: MockUserOwner.id, username: "alice", name: "Alice" },
			},
			{
				...mockDisabledSchedule,
				owner: { id: MockUserOwner.id, username: "alice", name: "Alice" },
			},
			mockWebhookAutomation,
		],
	},
};

export const MobileLayout: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
	},
};

export const Empty: Story = {
	args: { automations: [] },
};

export const MissingTarget: Story = {
	args: {
		automations: [
			{
				...mockScheduleAutomation,
				id: "automation-no-target",
				name: "Deleted target",
				target_chat_id: undefined,
				target_chat: undefined,
			},
			{
				...mockScheduleAutomation,
				id: "automation-archived-target",
				name: "Archived target",
				target_chat_id: mockArchivedChat.id,
				target_chat: { id: mockArchivedChat.id, title: mockArchivedChat.title },
				paused_reasons: ["target_unavailable"],
				next_run_times: [],
			},
		],
	},
};

export const RunNowError: Story = {
	args: {
		runError: {
			automation: mockScheduleAutomation,
			error: mockApiError({
				message: "The target chat is busy and the automation skips busy runs.",
			}),
		},
	},
};

export const ChatsDialog: Story = {
	args: {
		chatsDialog: {
			automation: mockScheduleAutomation,
			chats: [mockTargetChat, mockCreatingChat, mockArchivedChat],
			isLoading: false,
			error: undefined,
			hasNextPage: true,
			isFetchingNextPage: false,
			onLoadMore: fn(),
			onClose: fn(),
		},
	},
};

export const ChatsDialogLoadMoreError: Story = {
	args: {
		chatsDialog: {
			automation: mockScheduleAutomation,
			chats: [mockTargetChat, mockCreatingChat],
			isLoading: false,
			error: mockApiError({ message: "Failed to load more chats." }),
			hasNextPage: true,
			isFetchingNextPage: false,
			onLoadMore: fn(),
			onClose: fn(),
		},
	},
};

export const Loading: Story = {
	args: { automations: undefined, isLoading: true },
};

export const RefetchError: Story = {
	args: {
		error: mockApiError({ message: "Failed to refresh chat automations." }),
	},
};

export const ListError: Story = {
	args: {
		automations: undefined,
		error: mockApiError({ message: "Failed to list chat automations." }),
	},
};

export const DeleteDialog: Story = {
	args: {
		deleteDialog: {
			automation: mockScheduleAutomation,
			isDeleting: false,
			error: undefined,
			onConfirm: fn(),
			onClose: fn(),
		},
	},
};

export const DeleteDialogError: Story = {
	args: {
		deleteDialog: {
			automation: mockWebhookAutomation,
			isDeleting: false,
			error: mockApiError({ message: "Could not delete the automation." }),
			onConfirm: fn(),
			onClose: fn(),
		},
	},
};
