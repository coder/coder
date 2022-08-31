import { Story } from "@storybook/react"
import { WorkspaceQuota, WorkspaceQuotaProps } from "./WorkspaceQuota"

export default {
  title: "components/WorkspaceQuota",
  component: WorkspaceQuota,
}

const Template: Story<WorkspaceQuotaProps> = (args) => <WorkspaceQuota {...args} />

export const Example = Template.bind({})
Example.args = {
  count: 1,
  limit: 3,
}

export const LimitOf1 = Template.bind({})
LimitOf1.args = {
  count: 1,
  limit: 1,
}

export const Loading = Template.bind({})
Loading.args = {
  count: undefined,
  limit: undefined,
}


