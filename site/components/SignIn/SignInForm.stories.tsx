import { Story } from "@storybook/react"
import React from "react"
import { BrowserRouter } from "react-router-dom"
import { SignInForm, SignInFormProps } from "./SignInForm"

export default {
  title: "SignIn/SignInForm",
  component: SignInForm,
  argTypes: {
    isSignedIn: "boolean",
    isLoading: "boolean",
    redirectTo: "string",
    authErrorMessage: "string",
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<SignInFormProps> = (args: SignInFormProps) => (
    <SignInForm {...args} />
)

export const SignedOut = Template.bind({})
SignedOut.args = {
  isSignedIn: false,
  isLoading: false,
  redirectTo: "/projects",
  authErrorMessage: undefined,
  onSubmit: ({ email, password }) => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = { ...SignedOut.args, isLoading: true }

export const WithError = Template.bind({})
WithError.args = { ...SignedOut.args, authErrorMessage: "Email or password was invalid" }