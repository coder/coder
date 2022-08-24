import { ComponentMeta, Story } from "@storybook/react"
import { MockWorkspaceBuildLogs } from "../../testHelpers/entities"
import { Logs, LogsProps } from "./Logs"

export default {
  title: "components/Logs",
  component: Logs,
} as ComponentMeta<typeof Logs>

const Template: Story<LogsProps> = (args) => <Logs {...args} />

const lines = MockWorkspaceBuildLogs.map((log) => ({
  time: log.created_at,
  output: log.output,
}))
export const Example = Template.bind({})
Example.args = {
  lines,
}
