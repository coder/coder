import { ComponentMeta, Story } from "@storybook/react";
import { DeploymentOption } from "api/types";
import {
  SecuritySettingsPageView,
  SecuritySettingsPageViewProps,
} from "./SecuritySettingsPageView";

export default {
  title: "pages/SecuritySettingsPageView",
  component: SecuritySettingsPageView,
  args: {
    options: [
      {
        name: "SSH Keygen Algorithm",
        description: "something",
        value: "1234",
      },
      {
        name: "Secure Auth Cookie",
        description: "something",
        value: "1234",
      },
      {
        name: "Disable Owner Workspace Access",
        description: "something",
        value: false,
      },
      {
        name: "TLS Version",
        description: "something",
        value: ["something"],
        group: {
          name: "TLS",
        },
      },
    ],
    featureAuditLogEnabled: true,
    featureBrowserOnlyEnabled: true,
  },
} as ComponentMeta<typeof SecuritySettingsPageView>;

const Template: Story<SecuritySettingsPageViewProps> = (args) => (
  <SecuritySettingsPageView {...args} />
);
export const Page = Template.bind({});

export const NoTLS = Template.bind({});
NoTLS.args = {
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
};
