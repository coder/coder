import { ComponentMeta, Story } from "@storybook/react"
import { TableRowMenu, TableRowMenuProps } from "./TableRowMenu"

export default {
  title: "components/TableRowMenu",
  component: TableRowMenu,
} as ComponentMeta<typeof TableRowMenu>

type DataType = {
  id: string
}

const Template: Story<TableRowMenuProps<DataType>> = (args) => (
  <TableRowMenu {...args} />
)

export const Example = Template.bind({})
Example.args = {
  data: { id: "123" },
  menuItems: [
    { label: "Suspend", onClick: (data) => alert(data.id) },
    { label: "Update", onClick: (data) => alert(data.id) },
    { label: "Delete", onClick: (data) => alert(data.id) },
  ],
}
