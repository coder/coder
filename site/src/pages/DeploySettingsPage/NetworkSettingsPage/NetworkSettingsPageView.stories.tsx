import { ComponentMeta, Story } from "@storybook/react"
import {
  NetworkSettingsPageView,
  NetworkSettingsPageViewProps,
} from "./NetworkSettingsPageView"

export default {
  title: "pages/NetworkSettingsPageView",
  component: NetworkSettingsPageView,
  argTypes: {
    deploymentConfig: {
      defaultValue: {
        derp: {
          server: {
            enable: {
              name: "DERP Server Enable",
              usage:
                "Whether to enable or disable the embedded DERP relay server.",
              value: true,
            },
            region_name: {
              name: "DERP Server Region Name",
              usage: "Region name that for the embedded DERP server.",
              value: "aws-east",
            },
            stun_addresses: {
              name: "DERP Server STUN Addresses",
              usage:
                "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
              value: ["stun.l.google.com:19302", "stun.l.google.com:19301"],
            },
          },
          config: {
            url: {
              name: "DERP Config URL",
              usage:
                "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
              value: "https://coder.com",
            },
          },
        },
        wildcard_access_url: {
          value: "https://coder.com",
        },
      },
    },
  },
} as ComponentMeta<typeof NetworkSettingsPageView>

const Template: Story<NetworkSettingsPageViewProps> = (args) => (
  <NetworkSettingsPageView {...args} />
)
export const Page = Template.bind({})
