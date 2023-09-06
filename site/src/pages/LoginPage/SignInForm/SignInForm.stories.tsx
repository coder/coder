import { Story } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { SignInForm, SignInFormProps } from "./SignInForm";

export default {
  title: "components/SignInForm",
  component: SignInForm,
  argTypes: {
    isLoading: "boolean",
    onSubmit: { action: "Submit" },
  },
};

const Template: Story<SignInFormProps> = (args: SignInFormProps) => (
  <SignInForm {...args} />
);

export const SignedOut = Template.bind({});
SignedOut.args = {
  isSigningIn: false,
  onSubmit: () => {
    return Promise.resolve();
  },
};

export const SigningIn = Template.bind({});
SigningIn.args = {
  ...SignedOut.args,
  isSigningIn: true,
  authMethods: {
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
};

export const WithError = Template.bind({});
WithError.args = {
  ...SignedOut.args,
  error: mockApiError({
    message: "Email or password was invalid",
    validations: [
      {
        field: "password",
        detail: "Password is invalid.",
      },
    ],
  }),
  initialTouched: {
    password: true,
  },
};

export const WithGithub = Template.bind({});
WithGithub.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
};

export const WithOIDC = Template.bind({});
WithOIDC.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: false },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
};

export const WithOIDCWithoutPassword = Template.bind({});
WithOIDCWithoutPassword.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: false },
    github: { enabled: false },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
};

export const WithoutAny = Template.bind({});
WithoutAny.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: false },
    github: { enabled: false },
    oidc: { enabled: false, signInText: "", iconUrl: "" },
  },
};

export const WithGithubAndOIDC = Template.bind({});
WithGithubAndOIDC.args = {
  ...SignedOut.args,
  authMethods: {
    password: { enabled: true },
    github: { enabled: true },
    oidc: { enabled: true, signInText: "", iconUrl: "" },
  },
};
