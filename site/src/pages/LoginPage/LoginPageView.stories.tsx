import {
	MockAuthMethodsAll,
	MockAuthMethodsExternal,
	MockAuthMethodsPasswordOnly,
	MockAuthMethodsPasswordTermsOfService,
	MockBuildInfo,
	mockApiError,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { LoginPageView } from "./LoginPageView";

const meta: Meta<typeof LoginPageView> = {
	title: "pages/LoginPage",
	component: LoginPageView,
	args: {
		buildInfo: MockBuildInfo,
	},
};

export default meta;
type Story = StoryObj<typeof LoginPageView>;

export const Example: Story = {
	args: {
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const WithExternalAuthMethods: Story = {
	args: {
		authMethods: MockAuthMethodsExternal,
	},
};

export const WithAllAuthMethods: Story = {
	args: {
		authMethods: MockAuthMethodsAll,
	},
};

export const WithTermsOfService: Story = {
	args: {
		authMethods: MockAuthMethodsPasswordTermsOfService,
	},
};

export const AuthError: Story = {
	args: {
		error: mockApiError({
			message: "Incorrect email or password.",
		}),
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const ExternalAuthError: Story = {
	args: {
		error: mockApiError({
			message: "Incorrect email or password.",
		}),
		authMethods: MockAuthMethodsAll,
	},
};

export const LoadingAuthMethods: Story = {
	args: {
		isLoading: true,
		authMethods: undefined,
	},
};

export const SigningIn: Story = {
	args: {
		isSigningIn: true,
		authMethods: MockAuthMethodsPasswordOnly,
	},
};

export const WithFieldValidation: Story = {
	args: {
		authMethods: MockAuthMethodsPasswordOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await user.click(canvas.getByRole("button", { name: /sign in/i }));
	},
};

export const WithInvalidEmail: Story = {
	args: {
		authMethods: MockAuthMethodsPasswordOnly,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const emailInput = await canvas.findByLabelText(/email/i);
		await user.type(emailInput, "not-an-email");
		await user.click(canvas.getByRole("button", { name: /sign in/i }));
	},
};
