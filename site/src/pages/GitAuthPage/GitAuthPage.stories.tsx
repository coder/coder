import { ComponentMeta, Story } from "@storybook/react"
import GitAuthPage from "./GitAuthPage"

export default {
  title: "pages/GitAuthPage",
  component: GitAuthPage,
} as ComponentMeta<typeof GitAuthPage>

const Template: Story = (args) => <GitAuthPage {...args} />

export const Default = Template.bind({})
Default.args = {}
