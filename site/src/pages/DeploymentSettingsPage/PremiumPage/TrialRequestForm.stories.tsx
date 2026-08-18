import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { TrialRequestForm } from "./TrialRequestForm";

const meta: Meta<typeof TrialRequestForm> = {
	title: "pages/DeploymentSettingsPage/TrialRequestForm",
	component: TrialRequestForm,
	args: {
		isSubmitting: false,
		onSubmit: fn(),
	},
};

export default meta;

type Story = StoryObj<typeof TrialRequestForm>;

const COMPLETE_REQUEST = {
	email: "coder@coder.com",
	first_name: "Coder",
	last_name: "McCoder",
	phone_number: "+14155552671",
	job_title: "Platform Engineer",
	company_name: "Coder",
	country: "United States",
	developers: "51 - 100",
};

/** Radix Select portals its listbox into document.body. */
const selectOption = async (
	canvasElement: HTMLElement,
	comboboxName: RegExp,
	optionName: RegExp,
) => {
	const canvas = within(canvasElement);
	const body = within(canvasElement.ownerDocument.body);

	await userEvent.click(
		await canvas.findByRole("combobox", { name: comboboxName }),
	);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const submit = canvas.getByRole("button", { name: "Start a trial" });

		await expect(submit).toBeDisabled();

		await userEvent.click(canvas.getByRole("checkbox"));

		await waitFor(() => expect(submit).toBeEnabled());
		// The gating hint disappears once the box is checked.
		await expect(
			canvas.queryByText(
				"Acknowledge the database requirements to start your trial.",
			),
		).not.toBeInTheDocument();
	},
};

export const ValidationErrors: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText("Please enter an email address."),
			).toBeInTheDocument(),
		);
		await expect(
			canvas.getByText("Please enter your first name."),
		).toBeInTheDocument();
		await expect(
			canvas.getByText("Please select your country."),
		).toBeInTheDocument();
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const InvalidEmail: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.type(
			canvas.getByLabelText(/^Business email/),
			"not-an-email",
		);
		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText("Please enter a valid email address."),
			).toBeInTheDocument(),
		);
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const InvalidPhoneNumber: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.type(
			canvas.getByLabelText(/^Phone number/),
			"415-555-CODE",
		);
		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText(
					"Phone number should be in international format (e.g. +14155552671).",
				),
			).toBeInTheDocument(),
		);
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const TooShortJobTitle: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.type(canvas.getByLabelText(/^Job title/), "X");
		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		await waitFor(() =>
			expect(
				canvas.getByText("Job title should be at least 2 characters."),
			).toBeInTheDocument(),
		);
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

// The character counter is driven by `maxLength` in getFormHelpers, which starts
// warning 30 characters before the limit and turns into an error past it.
export const CompanyNameOverLimit: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.type(canvas.getByLabelText(/^Company/), "a".repeat(101));

		await waitFor(() =>
			expect(
				canvas.getByText(
					"This cannot be longer than 100 characters. (101/100)",
				),
			).toBeInTheDocument(),
		);

		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

// Pins the "Number of developers" bucket list.
export const SelectDevelopers: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await canvas.findByRole("combobox", { name: /^Number of developers/ }),
		);

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

export const SubmitsCompleteForm: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.type(
			canvas.getByLabelText(/^Business email/),
			COMPLETE_REQUEST.email,
		);
		await userEvent.type(
			canvas.getByLabelText(/^First name/),
			COMPLETE_REQUEST.first_name,
		);
		await userEvent.type(
			canvas.getByLabelText(/^Last name/),
			COMPLETE_REQUEST.last_name,
		);
		await userEvent.type(
			canvas.getByLabelText(/^Company/),
			COMPLETE_REQUEST.company_name,
		);
		await userEvent.type(
			canvas.getByLabelText(/^Job title/),
			COMPLETE_REQUEST.job_title,
		);
		await userEvent.type(
			canvas.getByLabelText(/^Phone number/),
			COMPLETE_REQUEST.phone_number,
		);
		await selectOption(canvasElement, /^Number of developers/, /^51 - 100$/);
		await selectOption(canvasElement, /^Country/, /United States$/);

		await userEvent.click(canvas.getByRole("checkbox"));
		await userEvent.click(
			canvas.getByRole("button", { name: "Start a trial" }),
		);

		// Deep equality, so an extra `acknowledged` key would fail this assertion.
		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(COMPLETE_REQUEST),
		);
	},
};

export const KeyboardNavigation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		canvas.getByLabelText(/^Phone number/).focus();

		const tabOrder = [
			canvas.getByRole("combobox", { name: /^Number of developers/ }),
			canvas.getByRole("combobox", { name: /^Country/ }),
			canvas.getByRole("checkbox"),
			canvas.getByRole("link", { name: "Learn more" }),
		];

		for (const element of tabOrder) {
			await userEvent.tab();
			await expect(element).toHaveFocus();
		}

		// The docs link sits outside the checkbox label, so reaching it leaves
		// the acknowledgement untouched.
		await expect(canvas.getByRole("checkbox")).not.toBeChecked();
	},
};

export const ServerError: Story = {
	args: {
		error: mockApiError({
			message: "Unable to issue a trial license.",
			validations: [
				{ field: "email", detail: "This email already started a trial." },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(canvas.getByLabelText(/^Business email/));
		await userEvent.tab();

		await waitFor(() =>
			expect(
				canvas.getByText("This email already started a trial."),
			).toBeInTheDocument(),
		);
	},
};

export const Submitting: Story = {
	args: {
		isSubmitting: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: /Start a trial/ }),
		).toBeDisabled();
		await expect(canvas.getByLabelText(/^Business email/)).toBeDisabled();
	},
};
