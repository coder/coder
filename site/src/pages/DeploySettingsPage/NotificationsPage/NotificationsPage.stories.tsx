import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import {
  notificationDispatchMethodsKey,
  systemNotificationTemplatesKey,
} from "api/queries/notifications";
import type { DeploymentValues } from "api/typesGenerated";
import {
  MockNotificationMethodsResponse,
  MockNotificationTemplates,
  MockUser,
} from "testHelpers/entities";
import {
  withAuthProvider,
  withDashboardProvider,
  withDeploySettings,
  withGlobalSnackbar,
} from "testHelpers/storybook";
import { NotificationsPage } from "./NotificationsPage";

const meta: Meta<typeof NotificationsPage> = {
  title: "pages/DeploymentSettings/NotificationsPage",
  component: NotificationsPage,
  parameters: {
    experiments: ["notifications"],
    queries: [
      { key: systemNotificationTemplatesKey, data: MockNotificationTemplates },
      {
        key: notificationDispatchMethodsKey,
        data: MockNotificationMethodsResponse,
      },
    ],
    user: MockUser,
    permissions: { viewDeploymentValues: true },
    deploymentValues: {
      notifications: {
        webhook: {
          endpoint: "https://example.com",
        },
      },
    } as DeploymentValues,
  },
  decorators: [
    withGlobalSnackbar,
    withAuthProvider,
    withDashboardProvider,
    withDeploySettings,
  ],
};

export default meta;

type Story = StoryObj<typeof NotificationsPage>;

export const Default: Story = {};

export const Toggle: Story = {
  play: async ({ canvasElement }) => {
    spyOn(API, "updateNotificationTemplateMethod").mockResolvedValue();
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const option = await canvas.findByText("Workspace Marked as Dormant");
    const toggleButton = within(option.closest("li")!).getByRole("button", {
      name: "Webhook",
    });
    await user.click(toggleButton);
  },
};
