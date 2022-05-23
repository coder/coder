import { Story } from "@storybook/react"
import React from "react"
import { MockWorkspace } from "../../testHelpers/renderHelpers"
import { WorkspaceSchedule, WorkspaceScheduleProps } from "./WorkspaceSchedule"

export default {
  title: "components/WorkspaceSchedule",
  component: WorkspaceSchedule,
  argTypes: {},
}

const Template: Story<WorkspaceScheduleProps> = (args) => <WorkspaceSchedule {...args} />

export const Example = Template.bind({})
Example.args = {
  workspace: MockWorkspace,
}
