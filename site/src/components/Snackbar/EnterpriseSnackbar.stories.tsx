import { Story } from "@storybook/react"
import React from "react"
import { EnterpriseSnackbar, EnterpriseSnackbarProps } from "./EnterpriseSnackbar"

export default {
  title: "Snackbar/EnterpriseSnackbar",
  component: EnterpriseSnackbar,
}

const Template: Story<EnterpriseSnackbarProps> = (args: EnterpriseSnackbarProps) => <EnterpriseSnackbar {...args} />

export const Error = Template.bind({})
Error.args = {
  variant: "error",
  open: true,
  message: "Oops, something wrong happened.",
}

export const Info = Template.bind({})
Info.args = {
  variant: "info",
  open: true,
  message: "Hey, something happened.",
}
