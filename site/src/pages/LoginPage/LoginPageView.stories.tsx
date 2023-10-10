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
    authMethods: MockAuthMethods,
  },
};

export const AuthError: Story = {
  args: {
    isLoading: false,
    error: mockApiError({
      message: "User or password is incorrect",
      detail: "Please, try again",
    }),
    authMethods: MockAuthMethods,
  },
};

export const LoadingInitialData: Story = {
  args: {
    isLoading: true,
  },
};

export const SigningIn: Story = {
  args: {
    isSigningIn: true,
    authMethods: MockAuthMethods,
  },
};
