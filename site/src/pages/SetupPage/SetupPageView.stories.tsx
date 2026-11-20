import { chromatic } from "testHelpers/chromatic";
import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { SetupPageView } from "./SetupPageView";

const meta: Meta<typeof SetupPageView> = {
	title: "pages/SetupPage",
	parameters: { chromatic },
	component: SetupPageView,
};

export default meta;
type Story = StoryObj<typeof SetupPageView>;

export const Ready: Story = {};

export const SetupTokenRequired: Story = {
	args: {
		authMethods: {
			github: { enabled: true, default_provider_configured: true },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
			password: { enabled: true },
			setup_token_required: true,
		},
	},
};

export const SetupTokenRefused: Story = {
	args: {
		...SetupTokenRequired.args,
		onSubmit: fn(),
		error: mockApiError({
			message:
				"A setup token is required to create the first user. Find it in the output of coder server.",
			validations: [
				{ field: "setup_token", detail: "Missing or incorrect setup token." },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Setup token"), "wrong-token");
		await userEvent.type(canvas.getByLabelText("Email"), "sam@coder.com");
		await userEvent.type(canvas.getByLabelText("Password"), "s3cure&password!");
		await userEvent.click(canvas.getByTestId("create"));
	},
};

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
