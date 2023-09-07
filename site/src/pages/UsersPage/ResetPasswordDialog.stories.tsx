import { action } from "@storybook/addon-actions";
import { Story } from "@storybook/react";
import { MockUser } from "testHelpers/entities";

import {
  ResetPasswordDialog,
  ResetPasswordDialogProps,
} from "./ResetPasswordDialog";

export default {
  title: "components/Dialogs/ResetPasswordDialog",
  component: ResetPasswordDialog,
  argTypes: {
    onClose: { action: "onClose", defaultValue: action("onClose") },
    onConfirm: { action: "onConfirm", defaultValue: action("onConfirm") },
  },
};

const Template: Story<ResetPasswordDialogProps> = (
  args: ResetPasswordDialogProps,
) => <ResetPasswordDialog {...args} />;

export const Example = Template.bind({});
Example.args = {
  open: true,
  user: MockUser,
  newPassword: "somerandomstringhere",
};
