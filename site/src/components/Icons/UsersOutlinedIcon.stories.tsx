import { Story } from "@storybook/react"
import { UsersOutlinedIcon } from "./UsersOutlinedIcon"

export default {
  title: "icons/UsersOutlinedIcon",
  component: UsersOutlinedIcon,
}

const Template: Story = (args) => <UsersOutlinedIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
