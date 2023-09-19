import { action } from "@storybook/addon-actions";
import { MockAuthMethods, mockApiError } from "testHelpers/entities";
import { LoginPageView } from "./LoginPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof LoginPageView> = {
  title: "pages/LoginPageView",
  component: LoginPageView,
};

export default meta;
type Story = StoryObj<typeof LoginPageView>;

export const Example: Story = {
  args: {
    isLoading: false,
    onSignIn: action("onSignIn"),
    context: {
      data: {
        authMethods: MockAuthMethods,
        hasFirstUser: false,
      },
    },
  },
};

export const AuthError: Story = {
  args: {
    isLoading: false,
    onSignIn: action("onSignIn"),
    context: {
      error: mockApiError({
        message: "User or password is incorrect",
        detail: "Please, try again",
      }),
      data: {
        authMethods: MockAuthMethods,
        hasFirstUser: false,
      },
    },
  },
};

export const LoadingInitialData: Story = {
  args: {
    isLoading: true,
    onSignIn: action("onSignIn"),
    context: {},
  },
};

export const SigningIn: Story = {
  args: {
    isSigningIn: true,
    onSignIn: action("onSignIn"),
    context: {
      data: {
        authMethods: MockAuthMethods,
        hasFirstUser: false,
      },
    },
  },
};
