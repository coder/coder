import { Story } from "@storybook/react"
import React from "react"
import { SignInForm, SignInProps } from "./SignInForm"

export default {
  title: "SignIn/SignInForm",
  component: SignInForm,
  argTypes: {
    loginHandler: { action: "Login" },
  },
}

const Template: Story<SignInProps> = (args) => <SignInForm {...args} />

export const Example = Template.bind({})
Example.args = {}
