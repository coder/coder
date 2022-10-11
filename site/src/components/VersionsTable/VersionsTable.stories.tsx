import { ComponentMeta, Story } from "@storybook/react"
import { MockTemplateVersion } from "../../testHelpers/entities"
import { VersionsTable, VersionsTableProps } from "./VersionsTable"

export default {
  title: "components/VersionsTable",
  component: VersionsTable,
} as ComponentMeta<typeof VersionsTable>

const Template: Story<VersionsTableProps> = (args) => (
  <VersionsTable {...args} />
)

export const Example = Template.bind({})
Example.args = {
  versions: [
    MockTemplateVersion,
    {
      ...MockTemplateVersion,
      name: "test-template-version-2",
      created_at: "2022-05-18T18:39:01.382927298Z",
    },
  ],
}

export const Empty = Template.bind({})
Empty.args = {
  versions: [],
}
