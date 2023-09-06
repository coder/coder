import { ComponentMeta, Story } from "@storybook/react"
import { LogLevel } from "api/typesGenerated"
import { MockWorkspaceBuildLogs } from "../../../testHelpers/entities"
import { Logs, LogsProps } from "./Logs"

export default {
  title: "components/Logs",
  component: Logs,
} as ComponentMeta<typeof Logs>

const Template: Story<LogsProps> = (args) => <Logs {...args} />

const lines = MockWorkspaceBuildLogs.map((log) => ({
  time: log.created_at,
  output: log.output,
  level: "info" as LogLevel,
}))
export const Example = Template.bind({})
Example.args = {
  lines,
}

export const WithLineNumbers = Template.bind({})
WithLineNumbers.args = {
  lines,
  lineNumbers: true,
}
