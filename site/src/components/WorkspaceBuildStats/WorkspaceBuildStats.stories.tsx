import { ComponentMeta, Story } from "@storybook/react"
import { MockWorkspaceBuild } from "../../testHelpers/entities"
import {
  WorkspaceBuildStats,
  WorkspaceBuildStatsProps,
} from "./WorkspaceBuildStats"

export default {
  title: "components/WorkspaceBuildStats",
  component: WorkspaceBuildStats,
} as ComponentMeta<typeof WorkspaceBuildStats>

const Template: Story<WorkspaceBuildStatsProps> = (args) => (
  <WorkspaceBuildStats {...args} />
)

export const Example = Template.bind({})
Example.args = {
  build: MockWorkspaceBuild,
}

export const Autostart = Template.bind({})
Autostart.args = {
  build: {
    ...MockWorkspaceBuild,
    reason: "autostart",
  },
}

export const Autostop = Template.bind({})
Autostop.args = {
  build: {
    ...MockWorkspaceBuild,
    transition: "stop",
    reason: "autostop",
  },
}
