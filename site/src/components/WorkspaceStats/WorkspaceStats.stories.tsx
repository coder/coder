import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/renderHelpers"
import { WorkspaceStats, WorkspaceStatsProps } from "../WorkspaceStats/WorkspaceStats"

export default {
  title: "components/WorkspaceStats",
  component: WorkspaceStats,
}

const Template: Story<WorkspaceStatsProps> = (args) => <WorkspaceStats {...args} />

export const Example = Template.bind({})
Example.args = {
  workspace: Mocks.MockWorkspace,
}
