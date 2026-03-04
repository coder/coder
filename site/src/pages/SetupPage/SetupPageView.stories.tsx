import { chromatic } from "testHelpers/chromatic";
import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { SetupPageView } from "./SetupPageView";

const meta: Meta<typeof SetupPageView> = {
	title: "pages/SetupPage",
	parameters: { chromatic },
	component: SetupPageView,
};

export default meta;
type Story = StoryObj<typeof SetupPageView>;

export const Ready: Story = {};

export const FormError: Story = {
	args: {
		error: mockApiError({
			validations: [{ field: "username", detail: "Username taken" }],
		}),
	},
};

export const TrialError: Story = {
	args: {
		error: mockApiError({
			message: "Couldn't generate trial!",
			detail: "It looks like your team is already trying Coder.",
		}),
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const WithGithubAuth: Story = {
	args: {
		authMethods: {
			github: {
				enabled: true,
				default_provider_configured: false,
			},
			password: {
				enabled: true,
			},
			oidc: {
				enabled: false,
				signInText: "",
				iconUrl: "",
			},
		},
	},
};

export const WithEnterpriseTrial: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trialCheckbox = await canvas.findByTestId("trial");
		await userEvent.click(trialCheckbox);
	},
};
