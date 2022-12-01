import { ComponentMeta, Story } from "@storybook/react"
import { UpdateCheckBanner, UpdateCheckBannerProps } from "./UpdateCheckBanner"

export default {
  title: "components/UpdateCheckBanner",
  component: UpdateCheckBanner,
} as ComponentMeta<typeof UpdateCheckBanner>

const Template: Story<UpdateCheckBannerProps> = (args) => (
  <UpdateCheckBanner {...args} />
)

export const UpdateAvailable = Template.bind({})
UpdateAvailable.args = {
  updateCheck: {
    current: false,
    version: "v0.12.9",
    url: "https://github.com/coder/coder/releases/tag/v0.12.9",
  },
}

export const UpdateCheckError = Template.bind({})
UpdateCheckError.args = {
  error: new Error("Something went wrong."),
}
