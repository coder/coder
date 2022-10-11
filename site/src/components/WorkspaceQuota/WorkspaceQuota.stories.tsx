import { Story } from "@storybook/react"
import { WorkspaceQuota, WorkspaceQuotaProps } from "./WorkspaceQuota"

export default {
  title: "components/WorkspaceQuota",
  component: WorkspaceQuota,
}

const Template: Story<WorkspaceQuotaProps> = (args) => (
  <WorkspaceQuota {...args} />
)

export const Example = Template.bind({})
Example.args = {
  quota: {
    user_workspace_count: 1,
    user_workspace_limit: 3,
  },
}

export const LimitOf1 = Template.bind({})
LimitOf1.args = {
  quota: {
    user_workspace_count: 1,
    user_workspace_limit: 1,
  },
}

export const Loading = Template.bind({})
Loading.args = {
  quota: undefined,
}

export const Error = Template.bind({})
Error.args = {
  quota: undefined,
  error: {
    response: {
      data: {
        message: "Failed to fetch workspace quotas!",
      },
    },
    isAxiosError: true,
  },
}

export const Disabled = Template.bind({})
Disabled.args = {
  quota: {
    user_workspace_count: 1,
    user_workspace_limit: 0,
  },
}
