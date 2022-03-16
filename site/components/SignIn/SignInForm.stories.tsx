import { Story } from "@storybook/react"
import React from "react"
import { SignInForm, SignInFormProps } from "./SignInForm"

export default {
  title: "SignIn/SignInForm",
  component: SignInForm,
  argTypes: {
    isLoading: "boolean",
    authErrorMessage: "string",
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<SignInFormProps> = (args: SignInFormProps) => <SignInForm {...args} />

export const SignedOut = Template.bind({})
SignedOut.args = {
  isLoading: false,
  authErrorMessage: undefined,
  onSubmit: () => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = { ...SignedOut.args, isLoading: true }

export const WithError = Template.bind({})
WithError.args = { ...SignedOut.args, authErrorMessage: "Email or password was invalid" }
