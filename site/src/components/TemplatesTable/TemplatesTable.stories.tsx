import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockOrganization, MockTemplate } from "../../testHelpers/entities"
import { TemplatesTable, TemplatesTableProps } from "./TemplatesTable"

export default {
  title: "components/TemplatesTable",
  component: TemplatesTable,
} as ComponentMeta<typeof TemplatesTable>

const Template: Story<TemplatesTableProps> = (args) => <TemplatesTable {...args} />

export const Example = Template.bind({})
Example.args = {
  templates: [MockTemplate],
  organizations: [MockOrganization],
}

export const Empty = Template.bind({})
Empty.args = {
  templates: [],
  organizations: [],
}

export const Loading = Template.bind({})
Loading.args = {
  templates: undefined,
  organizations: [],
}
