import { Meta, StoryObj } from "@storybook/react";
import { MockUser } from "testHelpers/entities";

import { ResetPasswordDialog } from "./ResetPasswordDialog";

const meta: Meta<typeof ResetPasswordDialog> = {
  title: "components/Dialogs/ResetPasswordDialog",
  component: ResetPasswordDialog,
};

export default meta;
type Story = StoryObj<typeof ResetPasswordDialog>;

export const Example: Story = {
  args: {
    open: true,
    user: MockUser,
    newPassword: "somerandomstringhere",
  },
};
