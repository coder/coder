import { ComponentMeta, Story } from "@storybook/react"
import {
  GitAuthSettingsPageView,
  GitAuthSettingsPageViewProps,
} from "./GitAuthSettingsPageView"

export default {
  title: "pages/GitAuthSettingsPageView",
  component: GitAuthSettingsPageView,
  argTypes: {
    deploymentConfig: {
      defaultValue: {
        gitauth: {
          name: "Git Auth",
          usage: "Automatically authenticate Git inside workspaces.",
          value: [
            {
              id: "123",
              client_id: "575",
            },
          ],
        },
      },
    },
  },
} as ComponentMeta<typeof GitAuthSettingsPageView>

const Template: Story<GitAuthSettingsPageViewProps> = (args) => (
  <GitAuthSettingsPageView {...args} />
)
export const Page = Template.bind({})
