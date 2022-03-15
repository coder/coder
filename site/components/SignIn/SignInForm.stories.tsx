import { Story } from "@storybook/react"
import React from "react"
import { BrowserRouter } from "react-router-dom"
import { SignInForm } from "./SignInForm"

export default {
  title: "SignIn/SignInForm",
  component: SignInForm,
  argTypes: {
    loginHandler: { action: "Login" },
  },
}

const Template: Story = (args) => (
  <BrowserRouter>
    <SignInForm {...args} />
  </BrowserRouter>
)

export const Example = Template.bind({})
Example.args = {}
