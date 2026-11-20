import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { chromatic } from "#/testHelpers/chromatic";
import { mockApiError } from "#/testHelpers/entities";
import { SetupPageView } from "./SetupPageView";

const meta: Meta<typeof SetupPageView> = {
	title: "pages/SetupPage",
	parameters: { chromatic },
	component: SetupPageView,
};

export default meta;
type Story = StoryObj<typeof SetupPageView>;

export const Ready: Story = {};

export const WithGitHub: Story = {
	args: {
		authMethods: {
			github: { enabled: true, default_provider_configured: false },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
			password: { enabled: true },
		},
	},
};

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

// TrialOpen pins the "Number of developers" bucket list. If this assertion
// changes, coordinate the new values with the licensor service owner, since
// the selected bucket is forwarded verbatim to v2-licensor.coder.com/trial.
export const TrialOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Radix Select portals its listbox into document.body.
		const body = within(canvasElement.ownerDocument.body);

		// Reveal the trial fields by checking the Premium trial checkbox.
		await userEvent.click(canvas.getByTestId("trial"));

		// Open the "Number of developers" Select.
		const trigger = await canvas.findByRole("combobox", {
			name: "Number of developers",
		});
		await userEvent.click(trigger);

		await waitFor(() => {
			const options = body.getAllByRole("option");
			expect(options.map((o) => o.textContent)).toEqual([
				"1 - 50",
				"51 - 100",
				"101 - 200",
				"201 - 500",
				"501 - 1000",
				"1001 - 2500",
				"2500+",
			]);
		});
	},
};
