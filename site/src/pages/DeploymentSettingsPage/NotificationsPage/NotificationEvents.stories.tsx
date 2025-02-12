import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import { selectTemplatesByGroup } from "api/queries/notifications";
import type { DeploymentValues } from "api/typesGenerated";
import { MockNotificationTemplates } from "testHelpers/entities";
import { NotificationEvents } from "./NotificationEvents";
import { baseMeta } from "./storybookUtils";

const meta: Meta<typeof NotificationEvents> = {
	title: "pages/DeploymentSettingsPage/NotificationsPage/NotificationEvents",
	component: NotificationEvents,
	args: {
		defaultMethod: "smtp",
		availableMethods: ["smtp", "webhook"],
		templatesByGroup: selectTemplatesByGroup(MockNotificationTemplates),
		deploymentConfig: baseMeta.parameters.deploymentValues,
	},
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof NotificationEvents>;

export const SMTPNotConfigured: Story = {
	args: {
		deploymentConfig: {
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

export const WebhookNotConfigured: Story = {
	args: {
		deploymentConfig: {
			notifications: {
				webhook: {
					endpoint: "",
				},
				email: {
					smarthost: "smtp.example.com",
					from: "bob@localhost",
					hello: "localhost",
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
		const tmpl = MockNotificationTemplates[4];
		const option = await canvas.findByText(tmpl.name);
		const li = option.closest("li");
		if (!li) {
			throw new Error("Could not find li");
		}
		const toggleButton = within(li).getByRole("button", {
			name: "Webhook",
		});
		await user.click(toggleButton);
		await within(document.body).findByText("Notification method updated");
	},
};

export const ToggleError: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "updateNotificationTemplateMethod").mockRejectedValue({});
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const tmpl = MockNotificationTemplates[4];
		const option = await canvas.findByText(tmpl.name);
		const li = option.closest("li");
		if (!li) {
			throw new Error("Could not find li");
		}
		const toggleButton = within(li).getByRole("button", {
			name: "Webhook",
		});
		await user.click(toggleButton);
		await within(document.body).findByText(
			"Failed to update notification method",
		);
	},
};
