import { Story } from "@storybook/react"
import { LoginErrors, SignInForm, SignInFormProps } from "./SignInForm"

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
  loginErrors: {},
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
  loginErrors: {
    [LoginErrors.AUTH_ERROR]: {
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
  },
  initialTouched: {
    password: true,
  },
}

export const WithCheckPermissionsError = Template.bind({})
WithCheckPermissionsError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.CHECK_PERMISSIONS_ERROR]: {
      response: {
        data: {
          message: "Unable to fetch user permissions",
          detail: "Resource not found or you do not have access to this resource.",
        },
      },
      isAxiosError: true,
    },
  },
}

export const WithAuthMethodsError = Template.bind({})
WithAuthMethodsError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.GET_METHODS_ERROR]: new Error("Failed to fetch auth methods"),
  },
}

export const WithGithub = Template.bind({})
WithGithub.args = {
  ...SignedOut.args,
  authMethods: {
    password: true,
    github: true,
  },
}
