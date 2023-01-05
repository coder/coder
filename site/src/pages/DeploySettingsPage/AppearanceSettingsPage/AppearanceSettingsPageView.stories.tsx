import { ComponentMeta, Story } from "@storybook/react"
import {
  AppearanceSettingsPageView,
  AppearanceSettingsPageViewProps,
} from "./AppearanceSettingsPageView"

export default {
  title: "pages/AppearanceSettingsPageView",
  component: AppearanceSettingsPageView,
  argTypes: {
    appearance: {
      defaultValue: {
        logo_url: "https://github.com/coder.png",
        service_banner: {
          enabled: true,
          message: "hello world",
          background_color: "white",
        },
      },
    },
    isEntitled: {
      defaultValue: false,
    },
    updateAppearance: {
      defaultValue: () => {
        return undefined
      },
    },
  },
} as ComponentMeta<typeof AppearanceSettingsPageView>

const Template: Story<AppearanceSettingsPageViewProps> = (args) => (
  <AppearanceSettingsPageView {...args} />
)
export const Page = Template.bind({})
