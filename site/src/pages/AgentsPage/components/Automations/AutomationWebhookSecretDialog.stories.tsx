import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockWebhookChatAutomation } from "#/testHelpers/chatEntities";
import {
	AutomationWebhookSecretDialog,
	webhookEventsUrl,
} from "./AutomationWebhookSecretDialog";

const meta: Meta<typeof AutomationWebhookSecretDialog> = {
	title: "pages/AgentsPage/Automations/AutomationWebhookSecretDialog",
	component: AutomationWebhookSecretDialog,
	args: {
		endpoint: webhookEventsUrl(
			"https://coder.example.com",
			MockWebhookChatAutomation.id,
		),
		secret: "cwhs_4f9a2c7e1b8d6a3f5e0c9b2d7a4e1f8c",
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AutomationWebhookSecretDialog>;

export const Default: Story = {};
