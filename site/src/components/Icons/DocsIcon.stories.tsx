import { Story } from "@storybook/react"
import { DocsIcon } from "./DocsIcon"

export default {
  title: "icons/DocsIcon",
  component: DocsIcon,
}

const Template: Story = (args) => <DocsIcon {...args} />

export const Example = Template.bind({})
Example.args = {}
