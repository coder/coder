import { action } from "@storybook/addon-actions"
import { ComponentMeta, Story } from "@storybook/react"
import { DeleteDialog, DeleteDialogProps } from "./DeleteDialog"

export default {
  title: "Components/Dialogs/DeleteDialog",
  component: DeleteDialog,
  argTypes: {
    onCancel: {
      action: "onClose",
      defaultValue: action("onClose"),
    },
    onConfirm: {
      action: "onConfirm",
      defaultValue: action("onConfirm"),
    },
    open: {
      control: "boolean",
      defaultValue: true,
    },
    entity: {
      defaultValue: "foo",
    },
    name: {
      defaultValue: "MyFoo",
    },
    info: {
      defaultValue:
        "Here's some info about the foo so you know you're deleting the right one.",
    },
  },
} as ComponentMeta<typeof DeleteDialog>

const Template: Story<DeleteDialogProps> = (args) => <DeleteDialog {...args} />

export const Example = Template.bind({})
Example.args = {
  isOpen: true,
}
