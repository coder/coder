import { Story } from "@storybook/react"
import { FormCloseButton, FormCloseButtonProps } from "./FormCloseButton"

export default {
  title: "components/FormCloseButton",
  component: FormCloseButton,
  argTypes: {
    onClose: { action: "onClose" },
  },
}

const Template: Story<FormCloseButtonProps> = (args) => (
  <FormCloseButton {...args} />
)

export const Example = Template.bind({})
Example.args = {}
