import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import type { DeploymentValues } from "api/typesGenerated";
import { baseMeta } from "./storybookUtils";
import { NotificationEvents } from "./NotificationEvents";
import { selectTemplatesByGroup } from "api/queries/notifications";
import { MockNotificationTemplates } from "testHelpers/entities";

const meta: Meta<typeof NotificationEvents> = {
	title: "pages/DeploymentSettings/NotificationsPage/NotificationEvents",
	component: NotificationEvents,
	args: {
		defaultMethod: "smtp",
		availableMethods: ["smtp", "webhook"],
		templatesByGroup: selectTemplatesByGroup(MockNotificationTemplates),
		deploymentValues: baseMeta.parameters.deploymentValues
	},
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof NotificationEvents>;

export const NoEmailSmarthost: Story = {
	args: {
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "https://example.com",
				},
				email: {
					smarthost: "",
				},
			},
		} as DeploymentValues,
	},
};

export const NoWebhookEndpoint: Story = {
	args: {
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "",
				},
				email: {
					smarthost: "smtp.example.com",
				},
			},
		} as DeploymentValues,
	},
};

export const Toggle: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "updateNotificationTemplateMethod").mockResolvedValue();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const option = await canvas.findByText("Workspace Marked as Dormant");
		const li = option.closest("li");
		if(!li) {
			throw new Error("Could not find li");
		}
		const toggleButton = within(li).getByRole("button", {
			name: "Webhook",
		});
		await user.click(toggleButton);
	},
};

