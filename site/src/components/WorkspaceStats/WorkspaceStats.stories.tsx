import { Story } from "@storybook/react"
import React from "react"
import * as Mocks from "../../testHelpers/renderHelpers"
import { WorkspaceStats, WorkspaceStatsProps } from "../WorkspaceStats/WorkspaceStats"

export default {
  title: "components/WorkspaceStats",
  component: WorkspaceStats,
}

const Template: Story<WorkspaceStatsProps> = (args) => <WorkspaceStats {...args} />

export const Start = Template.bind({})
Start.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "start",
    },
  },
}

export const Stop = Template.bind({})
Stop.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "stop",
    },
  },
}

export const Outdated = Template.bind({})
Outdated.args = {
  workspace: {
    ...Mocks.MockWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      transition: "start",
    },
    outdated: true,
  },
}
