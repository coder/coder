import { ComponentMeta, Story } from "@storybook/react"
import { DestructiveDialog, DestructiveDialogProps } from "./DestructiveDialog"

export default {
  title: "Components/Dialogs/DestructiveDialog",
  component: DestructiveDialog,
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
} as ComponentMeta<typeof DestructiveDialog>

const Template: Story<DestructiveDialogProps> = (args) => <DestructiveDialog {...args} />

export const Example = Template.bind({})
Example.args = {
  isOpen: true,
}
