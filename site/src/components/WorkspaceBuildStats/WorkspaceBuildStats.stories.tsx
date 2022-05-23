import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockWorkspaceBuild } from "../../testHelpers/entities"
import { WorkspaceBuildStats, WorkspaceBuildStatsProps } from "./WorkspaceBuildStats"

export default {
  title: "components/WorkspaceBuildStats",
  component: WorkspaceBuildStats,
} as ComponentMeta<typeof WorkspaceBuildStats>

const Template: Story<WorkspaceBuildStatsProps> = (args) => <WorkspaceBuildStats {...args} />

export const Example = Template.bind({})
Example.args = {
  build: MockWorkspaceBuild,
}
