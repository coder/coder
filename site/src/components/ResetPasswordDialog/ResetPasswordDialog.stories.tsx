import { Story } from "@storybook/react"
import { MockUser } from "../../testHelpers/renderHelpers"
import { ResetPasswordDialog, ResetPasswordDialogProps } from "./ResetPasswordDialog"

export default {
  title: "components/ResetPasswordDialog",
  component: ResetPasswordDialog,
  argTypes: {
    onClose: { action: "onClose" },
    onConfirm: { action: "onConfirm" },
  },
}

const Template: Story<ResetPasswordDialogProps> = (args: ResetPasswordDialogProps) => <ResetPasswordDialog {...args} />

export const Example = Template.bind({})
Example.args = {
  open: true,
  user: MockUser,
  newPassword: "somerandomstringhere",
}
