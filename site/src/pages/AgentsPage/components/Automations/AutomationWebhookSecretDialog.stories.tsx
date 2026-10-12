import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { webhookPublishEndpoint } from "#/api/queries/chatAutomations";
import { MockWebhookChatAutomation } from "#/testHelpers/chatEntities";
import { AutomationWebhookSecretDialog } from "./AutomationWebhookSecretDialog";

const meta: Meta<typeof AutomationWebhookSecretDialog> = {
	title: "pages/AgentsPage/Automations/AutomationWebhookSecretDialog",
	component: AutomationWebhookSecretDialog,
	args: {
		endpoint: webhookPublishEndpoint(
			"https://coder.example.com",
			MockWebhookChatAutomation.id,
		),
		secret: "cwhs_4f9a2c7e1b8d6a3f5e0c9b2d7a4e1f8c",
		returnFocusRef: { current: null },
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AutomationWebhookSecretDialog>;

export const Default: Story = {};
