import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { ConfirmDialog } from "./ConfirmDialog";

const meta: Meta<typeof ConfirmDialog> = {
  title: "components/Dialogs/ConfirmDialog",
  component: ConfirmDialog,
  args: {
    onClose: action("onClose"),
    onConfirm: action("onConfirm"),
    open: true,
    title: "Confirm Dialog",
  },
};

export default meta;
type Story = StoryObj<typeof ConfirmDialog>;

export const Example: Story = {
  args: {
    description: "Do you really want to delete me?",
    hideCancel: false,
    type: "delete",
  },
};

export const InfoDialog: Story = {
  args: {
    description: "Information is cool!",
    hideCancel: true,
    type: "info",
  },
};

export const InfoDialogWithCancel: Story = {
  args: {
    description: "Information can be cool!",
    hideCancel: false,
    type: "info",
  },
};

export const SuccessDialog: Story = {
  args: {
    description: "I am successful.",
    hideCancel: true,
    type: "success",
  },
};

export const SuccessDialogWithCancel: Story = {
  args: {
    description: "I may be successful.",
    hideCancel: false,
    type: "success",
  },
};

export const SuccessDialogLoading: Story = {
  args: {
    description: "I am successful.",
    hideCancel: true,
    type: "success",
    confirmLoading: true,
  },
};
