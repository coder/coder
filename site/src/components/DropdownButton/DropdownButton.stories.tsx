import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import {
  DeleteButton,
  DisabledButton,
  StartButton,
  UpdateButton,
} from "./ActionCtas"
import { DropdownButton, DropdownButtonProps } from "./DropdownButton"

export default {
  title: "Components/DropdownButton",
  component: DropdownButton,
}

const Template: Story<DropdownButtonProps> = (args) => (
  <DropdownButton {...args} />
)

export const WithDropdown = Template.bind({})
WithDropdown.args = {
  primaryAction: <StartButton handleAction={action("start")} />,
  secondaryActions: [
    {
      action: "update",
      button: <UpdateButton handleAction={action("update")} />,
    },
    {
      action: "delete",
      button: <DeleteButton handleAction={action("delete")} />,
    },
  ],
  canCancel: false,
}

export const WithCancel = Template.bind({})
WithCancel.args = {
  primaryAction: <DisabledButton label="deleting" />,
  secondaryActions: [],
  canCancel: true,
  handleCancel: action("cancel"),
}
