import { action } from "@storybook/addon-actions";
import { ComponentMeta, Story } from "@storybook/react";
import { ConfirmDialog, ConfirmDialogProps } from "./ConfirmDialog";

export default {
  title: "Components/Dialogs/ConfirmDialog",
  component: ConfirmDialog,
  argTypes: {
    open: {
      control: "boolean",
    },
  },
  args: {
    onClose: action("onClose"),
    onConfirm: action("onConfirm"),
    open: true,
    title: "Confirm Dialog",
  },
} as ComponentMeta<typeof ConfirmDialog>;

const Template: Story<ConfirmDialogProps> = (args) => (
  <ConfirmDialog {...args} />
);

export const DeleteDialog = Template.bind({});
DeleteDialog.args = {
  description: "Do you really want to delete me?",
  hideCancel: false,
  type: "delete",
};

export const InfoDialog = Template.bind({});
InfoDialog.args = {
  description: "Information is cool!",
  hideCancel: true,
  type: "info",
};

export const InfoDialogWithCancel = Template.bind({});
InfoDialogWithCancel.args = {
  description: "Information can be cool!",
  hideCancel: false,
  type: "info",
};

export const SuccessDialog = Template.bind({});
SuccessDialog.args = {
  description: "I am successful.",
  hideCancel: true,
  type: "success",
};

export const SuccessDialogWithCancel = Template.bind({});
SuccessDialogWithCancel.args = {
  description: "I may be successful.",
  hideCancel: false,
  type: "success",
};
