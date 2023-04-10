import { Story } from "@storybook/react"
import { MockWorkspace } from "testHelpers/entities"
import {
  WorkspaceStats,
  WorkspaceStatsProps,
} from "../WorkspaceStats/WorkspaceStats"

export default {
  title: "components/WorkspaceStats",
  component: WorkspaceStats,
}

const Template: Story<WorkspaceStatsProps> = (args) => (
  <WorkspaceStats {...args} />
)

export const Example = Template.bind({})
Example.args = {
  workspace: MockWorkspace,
}

export const Outdated = Template.bind({})
Outdated.args = {
  workspace: {
    ...MockWorkspace,
    outdated: true,
  },
}
