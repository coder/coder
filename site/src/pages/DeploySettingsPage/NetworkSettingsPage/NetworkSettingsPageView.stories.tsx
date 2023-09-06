import { ComponentMeta, Story } from "@storybook/react";
import {
  NetworkSettingsPageView,
  NetworkSettingsPageViewProps,
} from "./NetworkSettingsPageView";

export default {
  title: "pages/NetworkSettingsPageView",
  component: NetworkSettingsPageView,
  args: {
    options: [
      {
        name: "DERP Server Enable",
        usage: "Whether to enable or disable the embedded DERP relay server.",
        value: true,
        group: {
          name: "Networking",
        },
      },
      {
        name: "DERP Server Region Name",
        usage: "Region name that for the embedded DERP server.",
        value: "aws-east",
        group: {
          name: "Networking",
        },
      },
      {
        name: "DERP Server STUN Addresses",
        usage:
          "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
        value: ["stun.l.google.com:19302", "stun.l.google.com:19301"],
        group: {
          name: "Networking",
        },
      },
      {
        name: "DERP Config URL",
        usage:
          "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
        value: "https://coder.com",
        group: {
          name: "Networking",
        },
      },
      {
        name: "Wildcard Access URL",
        value: "https://coder.com",
        group: {
          name: "Networking",
        },
      },
    ],
  },
} as ComponentMeta<typeof NetworkSettingsPageView>;

const Template: Story<NetworkSettingsPageViewProps> = (args) => (
  <NetworkSettingsPageView {...args} />
);
export const Page = Template.bind({});
