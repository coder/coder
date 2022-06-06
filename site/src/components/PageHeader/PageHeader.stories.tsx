import { ComponentMeta, Story } from "@storybook/react"
import { PageHeader, PageHeaderTitle } from "./PageHeader"

export default {
  title: "components/PageHeader",
  component: PageHeader,
} as ComponentMeta<typeof PageHeader>

const Template: Story = () => (
  <PageHeader>
    <PageHeaderTitle>Templates</PageHeaderTitle>
  </PageHeader>
)

export const Example = Template.bind({})
