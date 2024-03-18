import type { Meta, StoryObj } from "@storybook/react";
import type { SerpentGroup, SerpentOption } from "api/typesGenerated";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const group: SerpentGroup = {
  name: "Networking",
  description: "",
};

const meta: Meta<typeof SecuritySettingsPageView> = {
  title: "pages/DeploySettingsPage/SecuritySettingsPageView",
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
      } as SerpentOption,
      {
        name: "Disable Owner Workspace Access",
        value: false,
      } as SerpentOption,
      {
        name: "Secure Auth Cookie",
        value: "1234",
      } as SerpentOption,
    ],
  },
};
