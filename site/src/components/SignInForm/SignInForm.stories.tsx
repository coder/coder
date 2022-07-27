import { Story } from "@storybook/react"
import { SignInForm, SignInFormProps } from "./SignInForm"

export default {
  title: "components/SignInForm",
  component: SignInForm,
  argTypes: {
    isLoading: "boolean",
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<SignInFormProps> = (args: SignInFormProps) => <SignInForm {...args} />

export const SignedOut = Template.bind({})
SignedOut.args = {
  isLoading: false,
  authError: undefined,
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
WithLoginError.args = {
  ...SignedOut.args,
  authError: {
    response: {
      data: {
        message: "Email or password was invalid",
        validations: [
          {
            field: "password",
            detail: "Password is invalid.",
          },
        ],
      },
    },
    isAxiosError: true,
  },
  initialTouched: {
    password: true,
  },
}

export const WithAuthMethodsError = Template.bind({})
WithAuthMethodsError.args = {
  ...SignedOut.args,
  methodsError: new Error("Failed to fetch auth methods"),
}

export const WithGithub = Template.bind({})
WithGithub.args = {
  ...SignedOut.args,
  authMethods: {
    password: true,
    github: true,
  },
}
