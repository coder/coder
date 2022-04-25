import { Story } from "@storybook/react"
import React from "react"
import { SignInForm, SignInFormProps } from "./SignInForm"

export default {
  title: "components/SignInForm",
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
Loading.args = {
  ...SignedOut.args,
  isLoading: true,
  authMethods: {
    github: true,
    password: true,
  },
}

export const WithLoginError = Template.bind({})
WithLoginError.args = { ...SignedOut.args, authErrorMessage: "Email or password was invalid" }

export const WithAuthMethodsError = Template.bind({})
WithAuthMethodsError.args = { ...SignedOut.args, methodsErrorMessage: "Failed to fetch auth methods" }

export const WithGithub = Template.bind({})
WithGithub.args = {
  ...SignedOut.args,
  authMethods: {
    password: true,
    github: true,
  },
}
