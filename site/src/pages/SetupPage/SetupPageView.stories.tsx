import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { SetupPageView, SetupPageViewProps } from "./SetupPageView"

export default {
  title: "pages/SetupPageView",
  component: SetupPageView,
}

const Template: Story<SetupPageViewProps> = (args: SetupPageViewProps) => (
  <SetupPageView {...args} />
)

export const Ready = Template.bind({})
Ready.args = {
  onSubmit: action("submit"),
  isCreating: false,
}

export const UnknownError = Template.bind({})
UnknownError.args = {
  onSubmit: action("submit"),
  isCreating: false,
  genericError: "Something went wrong",
}

export const FormError = Template.bind({})
FormError.args = {
  onSubmit: action("submit"),
  isCreating: false,
  formErrors: {
    username: "Username taken",
  },
}

export const Loading = Template.bind({})
Loading.args = {
  onSubmit: action("submit"),
  isCreating: true,
}
