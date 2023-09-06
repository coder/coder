import { action } from "@storybook/addon-actions";
import { ComponentMeta, Story } from "@storybook/react";
import { DeleteDialog, DeleteDialogProps } from "./DeleteDialog";

export default {
  title: "Components/Dialogs/DeleteDialog",
  component: DeleteDialog,
  argTypes: {
    open: {
      control: "boolean",
    },
  },
  args: {
    onCancel: action("onClose"),
    onConfirm: action("onConfirm"),
    open: true,
    entity: "foo",
    name: "MyFoo",
    info: "Here's some info about the foo so you know you're deleting the right one.",
  },
} as ComponentMeta<typeof DeleteDialog>;

const Template: Story<DeleteDialogProps> = (args) => <DeleteDialog {...args} />;

export const Example = Template.bind({});
Example.args = {
  isOpen: true,
};
