import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { chatEntityKey } from "#/api/queries/chats";
import type { Chat, ChatAutomation } from "#/api/typesGenerated";
import { MockChat, MockChatAutomation } from "#/testHelpers/chatEntities";
import { MockUserOwner, mockApiError } from "#/testHelpers/entities";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";

const targetChat: Chat = {
	...MockChat,
	id: "chat-target",
	title: "Nightly build triage",
};
const creatingChat: Chat = {
	...MockChat,
	id: "chat-creator",
	title: "Set up CI monitoring",
};
const archivedChat: Chat = {
	...MockChat,
	id: "chat-archived",
	title: "Old release thread",
	archived: true,
};

const scheduleAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-schedule",
	name: "Nightly build check",
	target_chat_id: targetChat.id,
	schedule_cron: "0 9 * * 1-5",
	schedule_time_zone: "Europe/Berlin",
	next_run_times: ["2026-10-01T07:00:00Z"],
};
const heartbeatAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "automation-heartbeat",
	name: "CI heartbeat",
	created_by_chat_id: creatingChat.id,
	target_chat_id: creatingChat.id,
	schedule_cron: "*/5 * * * *",
	schedule_time_zone: "UTC",
	next_run_times: ["2026-09-30T12:05:00Z"],
};
const webhookAutomation: ChatAutomation = {
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

const chatQueries = [targetChat, creatingChat, archivedChat].map((chat) => ({
	key: chatEntityKey(chat.id),
	data: chat,
}));

const meta: Meta<typeof AgentAutomationsPageView> = {
	title: "pages/AgentsPage/AgentAutomationsPageView",
	component: AgentAutomationsPageView,
	args: {
		currentUserId: MockUserOwner.id,
		organizationName: "Coder",
		automations: [scheduleAutomation, heartbeatAutomation, webhookAutomation],
		isLoading: false,
		error: undefined,
		onDismissRunError: fn(),
		onToggleEnabled: fn(),
		onRunNow: fn(),
		onViewChats: fn(),
	},
	parameters: { queries: chatQueries },
};

export default meta;
type Story = StoryObj<typeof AgentAutomationsPageView>;

export const WithAutomations: Story = {};

// Only the owner can run an automation, so another viewer gets no Run now.
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
				...scheduleAutomation,
				id: "automation-no-target",
				name: "Deleted target",
				target_chat_id: undefined,
			},
			{
				...scheduleAutomation,
				id: "automation-archived-target",
				name: "Archived target",
				target_chat_id: archivedChat.id,
			},
		],
	},
};

export const RunNowError: Story = {
	args: {
		runError: {
			automation: scheduleAutomation,
			error: mockApiError({
				message: "The target chat is busy and the automation skips busy runs.",
			}),
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
