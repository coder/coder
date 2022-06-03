import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { DeleteWorkspaceDialog, DeleteWorkspaceDialogProps } from "./DeleteWorkspaceDialog"

export default {
  title: "Components/DeleteWorkspaceDialog",
  component: DeleteWorkspaceDialog,
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
      defaultValue: "Confirm Dialog",
    },
  },
} as ComponentMeta<typeof DeleteWorkspaceDialog>

const Template: Story<DeleteWorkspaceDialogProps> = (args) => <DeleteWorkspaceDialog {...args} />

export const Example = Template.bind({})
Example.args = {
  isOpen: true,
}
