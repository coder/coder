import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockTemplate, MockWorkspace } from "../../testHelpers/entities"
import { WorkspacesTable, WorkspacesTableProps } from "./WorkspacesTable"

export default {
  title: "components/WorkspacesTable",
  component: WorkspacesTable,
} as ComponentMeta<typeof WorkspacesTable>

const Template: Story<WorkspacesTableProps> = (args) => <WorkspacesTable {...args} />

export const Example = Template.bind({})
Example.args = {
  templateInfo: MockTemplate,
  workspaces: [MockWorkspace],
  onCreateWorkspace: () => {
    console.info("Create workspace")
  },
}

export const Empty = Template.bind({})
Empty.args = {
  templateInfo: MockTemplate,
  workspaces: [],
  onCreateWorkspace: () => {
    console.info("Create workspace")
  },
}

export const Loading = Template.bind({})
Loading.args = {
  templateInfo: MockTemplate,
  workspaces: undefined,
  onCreateWorkspace: () => {
    console.info("Create workspace")
  },
}
