import { ComponentMeta, Story } from "@storybook/react"
import { MockWorkspaceBuildLogs } from "../../testHelpers/entities"
import {
  WorkspaceBuildLogs,
  WorkspaceBuildLogsProps,
} from "./WorkspaceBuildLogs"

export default {
  title: "components/WorkspaceBuildLogs",
  component: WorkspaceBuildLogs,
} as ComponentMeta<typeof WorkspaceBuildLogs>

const Template: Story<WorkspaceBuildLogsProps> = (args) => (
  <WorkspaceBuildLogs {...args} />
)

export const Example = Template.bind({})
Example.args = {
  logs: MockWorkspaceBuildLogs,
}
