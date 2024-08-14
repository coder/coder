import type { Meta, StoryObj } from "@storybook/react";
import type { SerpentGroup } from "api/typesGenerated";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const group: SerpentGroup = {
  name: "Networking",
  description: "",
};

const meta: Meta<typeof NetworkSettingsPageView> = {
  title: "pages/DeploySettingsPage/NetworkSettingsPageView",
  component: NetworkSettingsPageView,
  args: {
    options: [
      {
        name: "DERP Server Enable",
        description:
          "Whether to enable or disable the embedded DERP relay server.",
        value: true,
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "DERP Server Region Name",
        description: "Region name that for the embedded DERP server.",
        value: "aws-east",
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "DERP Server STUN Addresses",
        description:
          "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
        value: ["stun.l.google.com:19302", "stun.l.google.com:19301"],
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "DERP Config URL",
        description:
          "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
        value: "https://coder.com",
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
      {
        name: "Wildcard Access URL",
        description: "",
        value: "https://coder.com",
        group,
        flag: "derp",
        flag_shorthand: "d",
        hidden: false,
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof NetworkSettingsPageView>;

export const Page: Story = {};
