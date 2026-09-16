import type { Meta, StoryObj } from "@storybook/react-vite";
import { useFormik } from "formik";
import type { FC } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "./SelectField";

const regions = ["us-east-1", "us-west-2", "eu-central-1"];

interface ExampleSelectFieldProps {
	id?: string;
	label: string;
	description?: string;
	helperText?: string;
	required?: boolean;
	error?: string;
	value?: string;
	disabled?: boolean;
}

const ExampleSelectField: FC<ExampleSelectFieldProps> = ({
	id,
	label,
	description,
	helperText,
	required,
	error,
	value = "",
	disabled,
}) => {
	const form = useFormik({
		initialValues: { value },
		onSubmit: () => {},
	});

	return (
		<SelectField
			id={id}
			field={{
				name: "value",
				id: "value",
				value: form.values.value,
				onChange: form.handleChange,
				onBlur: form.handleBlur,
				error: Boolean(error),
				helperText: error ?? helperText,
			}}
			label={label}
			description={description}
			required={required}
			disabled={disabled}
			placeholder="Select a region..."
			onValueChange={(next) => form.setFieldValue("value", next)}
		>
			{regions.map((region) => (
				<SelectItem key={region} value={region}>
					{region}
				</SelectItem>
			))}
		</SelectField>
	);
};

const meta: Meta<typeof ExampleSelectField> = {
	title: "components/SelectField",
	component: ExampleSelectField,
	args: {
		id: "story-select",
		label: "Deployment region",
	},
};

export default meta;
type Story = StoryObj<typeof ExampleSelectField>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });
		await expect(trigger).not.toHaveAttribute("aria-describedby");
		await expect(trigger).not.toHaveAttribute("aria-invalid", "true");
		await expect(canvas.queryByText("*")).not.toBeInTheDocument();
		await expect(trigger).toHaveTextContent("Select a region...");
	},
};

export const Required: Story = {
	args: {
		required: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("*")).toBeVisible();
		await expect(
			canvas.getByRole("combobox", { name: /Deployment region/ }),
		).toHaveAttribute("aria-required", "true");
	},
};

export const WithDescription: Story = {
	args: {
		description: "Where new workspaces are provisioned.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });
		await expect(trigger).toHaveAttribute(
			"aria-describedby",
			"story-select-description",
		);
		await expect(
			canvas.getByText("Where new workspaces are provisioned."),
		).toHaveAttribute("id", "story-select-description");
	},
};

export const WithHelperText: Story = {
	args: {
		helperText: "This cannot be changed later.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });
		await expect(trigger).toHaveAttribute(
			"aria-describedby",
			"story-select-helper",
		);
	},
};

export const WithError: Story = {
	args: {
		error: "Please choose a region.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });
		await expect(trigger).toHaveAttribute(
			"aria-describedby",
			"story-select-error",
		);
		await expect(trigger).toHaveAttribute("aria-invalid", "true");
		await expect(canvas.getByText("Please choose a region.")).toBeVisible();
	},
};

export const WithDescriptionAndError: Story = {
	args: {
		description: "Where new workspaces are provisioned.",
		error: "Please choose a region.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });
		await expect(trigger).toHaveAttribute(
			"aria-describedby",
			"story-select-description story-select-error",
		);
		await expect(trigger).toHaveAttribute("aria-invalid", "true");
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: /Deployment region/ }),
		).toBeDisabled();
	},
};

export const SelectsOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Radix portals the listbox outside the story canvas.
		const body = within(canvasElement.ownerDocument.body);
		const trigger = canvas.getByRole("combobox", { name: /Deployment region/ });

		await userEvent.click(trigger);
		await waitFor(() =>
			expect(body.getAllByRole("option").map((o) => o.textContent)).toEqual(
				regions,
			),
		);

		await userEvent.click(body.getByRole("option", { name: "eu-central-1" }));
		await waitFor(() => expect(trigger).toHaveTextContent("eu-central-1"));
	},
};
