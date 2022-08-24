import { Story } from "@storybook/react"
import { BuildingIcon } from "./BuildingIcon"

export default {
  title: "icons/BuildingIcon",
  component: BuildingIcon,
}

const Template: Story = (args) => <BuildingIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
