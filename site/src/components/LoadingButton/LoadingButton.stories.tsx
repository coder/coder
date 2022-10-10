import { Story } from "@storybook/react"
import { LoadingButton, LoadingButtonProps } from "./LoadingButton"

export default {
  title: "components/LoadingButton",
  component: LoadingButton,
  argTypes: {
    loading: { control: { type: "boolean" } },
    children: { control: "text", defaultValue: "Create workspace" },
  },
}

const Template: Story<LoadingButtonProps> = (args) => (
  <LoadingButton {...args} />
)

export const Loading = Template.bind({})
Loading.args = {
  variant: "contained",
  loading: true,
}

export const NotLoading = Template.bind({})
NotLoading.args = {
  variant: "contained",
  loading: false,
}
