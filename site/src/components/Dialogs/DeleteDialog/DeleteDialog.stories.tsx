import { ComponentMeta, Story } from "@storybook/react"
import { DeleteDialog, DeleteDialogProps } from "./DeleteDialog"

export default {
  title: "Components/Dialogs/DeleteDialog",
  component: DeleteDialog,
  argTypes: {
    onClose: {
      action: "onClose",
    },
    onConfirm: {
      action: "onConfirm",
    },
    open: {
      control: "boolean",
      defaultValue: true,
    },
    title: {
      defaultValue: "Delete Something",
    },
    description: {
      defaultValue:
        "This is irreversible. To confirm, type the name of the thing you want to delete.",
    },
  },
} as ComponentMeta<typeof DeleteDialog>

const Template: Story<DeleteDialogProps> = (args) => <DeleteDialog {...args} />

export const Example = Template.bind({})
Example.args = {
  isOpen: true,
}
