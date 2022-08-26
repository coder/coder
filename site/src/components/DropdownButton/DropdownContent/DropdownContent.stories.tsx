import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { DeleteButton, UpdateButton, } from "../ActionCtas"
import { DropdownContent, DropdownContentProps } from "./DropdownContent"

export default {
  title: "DropdownButton/DropdownContent",
  component: DropdownContent,
}

const Template: Story<DropdownContentProps> = (args) => <DropdownContent {...args} />

export const Example = Template.bind({})
Example.args = {
  secondaryActions: [
    { action: "update", button: <UpdateButton handleAction={action("update")} /> },
    { action: "delete", button: <DeleteButton handleAction={action("delete")} /> }
  ]
}
