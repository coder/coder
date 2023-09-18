import { DeploymentGroup, DeploymentOption } from "api/api";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const group: DeploymentGroup = {
  name: "Networking",
  description: "",
  children: [] as DeploymentGroup[],
};

const meta: Meta<typeof SecuritySettingsPageView> = {
  title: "pages/SecuritySettingsPageView",
  component: SecuritySettingsPageView,
  args: {
    options: [
      {
        name: "SSH Keygen Algorithm",
        description: "something",
        value: "1234",
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "Secure Auth Cookie",
        description: "something",
        value: "1234",
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "Disable Owner Workspace Access",
        description: "something",
        value: false,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "TLS Version",
        description: "something",
        value: ["something"],
        group: { ...group, name: "TLS" },
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
    ],
    featureAuditLogEnabled: true,
    featureBrowserOnlyEnabled: true,
  },
};

export default meta;
type Story = StoryObj<typeof SecuritySettingsPageView>;

export const Page: Story = {};

export const NoTLS = {
  args: {
    options: [
      {
        name: "SSH Keygen Algorithm",
        value: "1234",
      } as DeploymentOption,
      {
        name: "Disable Owner Workspace Access",
        value: false,
      } as DeploymentOption,
      {
        name: "Secure Auth Cookie",
        value: "1234",
      } as DeploymentOption,
    ],
  },
};
