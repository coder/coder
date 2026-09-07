import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { selectTemplatesByGroup } from "#/api/queries/notifications";
import type { DeploymentValues } from "#/api/typesGenerated";
import { MockSystemNotificationTemplates } from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { NotificationEvents } from "./NotificationEvents";
import { baseMeta } from "./storybookUtils";

const meta: Meta<typeof NotificationEvents> = {
	title: "pages/DeploymentSettingsPage/NotificationsPage/NotificationEvents",
	component: NotificationEvents,
	args: {
		defaultMethod: "smtp",
		availableMethods: ["smtp", "webhook"],
		templatesByGroup: selectTemplatesByGroup(MockSystemNotificationTemplates),
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /View docs/ }),
		).toHaveAttribute(
			"href",
			docs("/admin/monitoring/notifications#smtp-email"),
		);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /View docs/ }),
		).toHaveAttribute("href", docs("/admin/monitoring/notifications#webhook"));
	},
};

export const ChangeMethod: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "updateNotificationTemplateMethod").mockResolvedValue();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const tmpl = MockSystemNotificationTemplates[4];
		const option = await canvas.findByText(tmpl.name);
		const row = option.closest("[data-testid='notification-template-row']");
		if (!(row instanceof HTMLElement)) {
			throw new Error("Could not find notification template row");
		}
		await user.click(
			within(row).getByRole("combobox", { name: /Notification method/ }),
		);
		await user.click(
			await within(document.body).findByRole("option", { name: "Webhook" }),
		);
		await within(document.body).findByText("Notification method updated.");
	},
};

export const ChangeMethodError: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "updateNotificationTemplateMethod").mockRejectedValue({});
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const tmpl = MockSystemNotificationTemplates[4];
		const option = await canvas.findByText(tmpl.name);
		const row = option.closest("[data-testid='notification-template-row']");
		if (!(row instanceof HTMLElement)) {
			throw new Error("Could not find notification template row");
		}
		await user.click(
			within(row).getByRole("combobox", { name: /Notification method/ }),
		);
		await user.click(
			await within(document.body).findByRole("option", { name: "Webhook" }),
		);
		await within(document.body).findByText(
			"Failed to update notification method.",
		);
	},
};
