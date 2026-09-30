import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { chatEntityKey } from "#/api/queries/chats";
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
	schedule_cron: "0 9 * * 1-5",
	schedule_time_zone: "Europe/Berlin",
	next_run_times: ["2026-10-01T07:00:00Z"],
};
const mockHeartbeatAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-heartbeat",
	name: "CI heartbeat",
	created_by_chat_id: mockCreatingChat.id,
	target_chat_id: mockCreatingChat.id,
	schedule_cron: "*/5 * * * *",
	schedule_time_zone: "UTC",
	next_run_times: ["2026-09-30T12:05:00Z"],
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

const meta: Meta<typeof AgentAutomationsPageView> = {
	title: "pages/AgentsPage/AgentAutomationsPageView",
	component: AgentAutomationsPageView,
	args: {
		currentUserId: MockUserOwner.id,
		organizationName: "Coder",
		automations: [
			mockScheduleAutomation,
			mockHeartbeatAutomation,
			mockWebhookAutomation,
		],
		isLoading: false,
		error: undefined,
		onDismissRunError: fn(),
		onToggleEnabled: fn(),
		onRunNow: fn(),
		onViewChats: fn(),
	},
	parameters: {
		queries: [
			{ key: chatEntityKey(mockTargetChat.id), data: mockTargetChat },
			{ key: chatEntityKey(mockCreatingChat.id), data: mockCreatingChat },
			{ key: chatEntityKey(mockArchivedChat.id), data: mockArchivedChat },
		],
	},
};

export default meta;
type Story = StoryObj<typeof AgentAutomationsPageView>;

export const WithAutomations: Story = {};

export const OtherOwner: Story = {
	args: { currentUserId: "another-user-id" },
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
			},
			{
				...mockScheduleAutomation,
				id: "automation-archived-target",
				name: "Archived target",
				target_chat_id: mockArchivedChat.id,
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
			chats: [mockTargetChat, mockCreatingChat],
			isLoading: false,
			error: undefined,
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

export const ListError: Story = {
	args: {
		automations: undefined,
		error: mockApiError({ message: "Failed to list chat automations." }),
	},
};
