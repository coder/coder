import { action } from "@storybook/addon-actions";
import { ComponentMeta, Story } from "@storybook/react";
import { MockAuthMethods } from "testHelpers/entities";
import { LoginPageView, LoginPageViewProps } from "./LoginPageView";

export default {
  title: "pages/LoginPageView",
  component: LoginPageView,
} as ComponentMeta<typeof LoginPageView>;

const Template: Story<LoginPageViewProps> = (args) => (
  <LoginPageView {...args} />
);

export const Example = Template.bind({});
Example.args = {
  isLoading: false,
  onSignIn: action("onSignIn"),
  context: {
    data: {
      authMethods: MockAuthMethods,
      hasFirstUser: false,
    },
  },
};

const err = new Error("Username or email are wrong.");

export const AuthError = Template.bind({});
AuthError.args = {
  isLoading: false,
  onSignIn: action("onSignIn"),
  context: {
    error: err,
    data: {
      authMethods: MockAuthMethods,
      hasFirstUser: false,
    },
  },
};

export const LoadingInitialData = Template.bind({});
LoadingInitialData.args = {
  isLoading: true,
  onSignIn: action("onSignIn"),
  context: {},
};

export const SigningIn = Template.bind({});
SigningIn.args = {
  isSigningIn: true,
  onSignIn: action("onSignIn"),
  context: {
    data: {
      authMethods: MockAuthMethods,
      hasFirstUser: false,
    },
  },
};
