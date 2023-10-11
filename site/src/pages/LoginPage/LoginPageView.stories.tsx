import { MockAuthMethods, mockApiError } from "testHelpers/entities";
import { LoginPageView } from "./LoginPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof LoginPageView> = {
  title: "pages/LoginPage",
  component: LoginPageView,
};

export default meta;
type Story = StoryObj<typeof LoginPageView>;

export const Example: Story = {
  args: {
    authMethods: MockAuthMethods,
  },
};

export const AuthError: Story = {
  args: {
    error: mockApiError({
      message: "User or password is incorrect",
      detail: "Please, try again",
    }),
    authMethods: MockAuthMethods,
  },
};

export const LoadingAuthMethods: Story = {
  args: {
    authMethods: undefined,
  },
};

export const SigningIn: Story = {
  args: {
    isSigningIn: true,
    authMethods: MockAuthMethods,
  },
};
