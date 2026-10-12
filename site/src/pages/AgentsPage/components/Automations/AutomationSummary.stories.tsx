import type { Meta, StoryObj } from "@storybook/react-vite";
import { organizationChatModelsKey } from "#/api/queries/chats";
import type { ChatAutomation } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import { MockChatModel } from "#/testHelpers/chatModels";
import { AutomationSummary } from "./AutomationSummary";

const mockScheduleAutomation: ChatAutomation = {
	...MockChatAutomation,
	schedule_cron: "0 9 * * 1-5",
	target_chat: { id: MockChat.id, title: "Release triage", archived: false },
};

const meta: Meta<typeof AutomationSummary> = {
	title: "pages/AgentsPage/Automations/AutomationSummary",
	component: AutomationSummary,
	args: {
		automation: mockScheduleAutomation,
		origin: "https://coder.example.com",
	},
};

export default meta;
type Story = StoryObj<typeof AutomationSummary>;

export const Schedule: Story = {};

export const Webhook: Story = {
	args: { automation: MockWebhookChatAutomation },
};

export const NewChat: Story = {
	args: {
		automation: {
			...MockChatAutomation,
			target_mode: "new_chat",
			target_chat_id: undefined,
			new_chat_model_config_id: MockChatModel.id,
			reasoning_effort: "high",
		},
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(MockChatAutomation.organization_id),
				data: {
					models: [MockChatModel],
					providers: [],
					unsupported_providers: [],
				},
			},
		],
	},
};
