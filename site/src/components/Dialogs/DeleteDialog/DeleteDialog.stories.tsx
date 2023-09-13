import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { DeleteDialog } from "./DeleteDialog";

const meta: Meta<typeof DeleteDialog> = {
  title: "components/Dialogs/DeleteDialog",
  component: DeleteDialog,
  args: {
    onCancel: action("onClose"),
    onConfirm: action("onConfirm"),
    isOpen: true,
    entity: "foo",
    name: "MyFoo",
    info: "Here's some info about the foo so you know you're deleting the right one.",
  },
};

export default meta;
type Story = StoryObj<typeof DeleteDialog>;

export const Example: Story = {};
