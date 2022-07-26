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
  },
  initialTouched: {
    password: true,
  },
}

export const WithGetUserError = Template.bind({})
WithGetUserError.args = {
  ...SignedOut.args,
  loginErrors: {
    getUserError: {
      response: {
        data: {
          message: "Unable to fetch user details",
          detail: "Resource not found or you do not have access to this resource.",
        },
      },
      isAxiosError: true,
    },
  },
}

export const WithCheckPermissionsError = Template.bind({})
WithCheckPermissionsError.args = {
  ...SignedOut.args,
  loginErrors: {
    checkPermissionsError: {
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
    getMethodsError: new Error("Failed to fetch auth methods"),
  },
}

export const WithGetUserAndAuthMethodsErrors = Template.bind({})
WithGetUserAndAuthMethodsErrors.args = {
  ...SignedOut.args,
  loginErrors: {
    getUserError: {
      response: {
        data: {
          message: "Unable to fetch user details",
          detail: "Resource not found or you do not have access to this resource.",
        },
      },
      isAxiosError: true,
    },
    getMethodsError: new Error("Failed to fetch auth methods"),
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
