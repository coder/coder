import { Story } from "@storybook/react"
import { makeMockApiError } from "testHelpers/entities"
import { LoginErrors, SignInForm, SignInFormProps } from "./SignInForm"

export default {
  title: "components/SignInForm",
  component: SignInForm,
  argTypes: {
    isLoading: "boolean",
    onSubmit: { action: "Submit" },
  },
}

const Template: Story<SignInFormProps> = (args: SignInFormProps) => (
  <SignInForm {...args} />
)

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
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
}

export const WithLoginError = Template.bind({})
WithLoginError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.AUTH_ERROR]: makeMockApiError({
      message: "Email or password was invalid",
      validations: [
        {
          field: "password",
          detail: "Password is invalid.",
        },
      ],
    }),
  },
  initialTouched: {
    password: true,
  },
}

export const WithGetUserError = Template.bind({})
WithGetUserError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.GET_USER_ERROR]: makeMockApiError({
      message: "You are logged out. Please log in to continue.",
      detail: "API Key is invalid.",
    }),
  },
}

export const WithCheckPermissionsError = Template.bind({})
WithCheckPermissionsError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.CHECK_PERMISSIONS_ERROR]: makeMockApiError({
      message: "Unable to fetch user permissions",
      detail: "Resource not found or you do not have access to this resource.",
    }),
  },
}

export const WithAuthMethodsError = Template.bind({})
WithAuthMethodsError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.GET_METHODS_ERROR]: new Error("Failed to fetch auth methods"),
  },
}

export const WithGetUserAndAuthMethodsError = Template.bind({})
WithGetUserAndAuthMethodsError.args = {
  ...SignedOut.args,
  loginErrors: {
    [LoginErrors.GET_USER_ERROR]: makeMockApiError({
      message: "You are logged out. Please log in to continue.",
      detail: "API Key is invalid.",
    }),
    [LoginErrors.GET_METHODS_ERROR]: new Error("Failed to fetch auth methods"),
  },
}

export const WithGithub = Template.bind({})
WithGithub.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
}

export const WithOIDC = Template.bind({})
WithOIDC.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: false },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
}

export const WithOIDCWithoutPassword = Template.bind({})
WithOIDCWithoutPassword.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: false },
    github: { enabled: false },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
}

export const WithoutAny = Template.bind({})
WithoutAny.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: false },
    github: { enabled: false },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
}

export const WithGithubAndOIDC = Template.bind({})
WithGithubAndOIDC.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
}
