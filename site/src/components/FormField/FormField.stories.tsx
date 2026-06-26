import type { Meta, StoryObj } from "@storybook/react-vite";
import { useFormik } from "formik";
import type { FC } from "react";
import { expect, within } from "storybook/test";
import { FormField } from "./FormField";

interface ExampleFormFieldProps {
	id?: string;
	label: string;
	description?: string;
	helperText?: string;
	required?: boolean;
	error?: string;
	value?: string;
}

const ExampleFormField: FC<ExampleFormFieldProps> = ({
	id,
	label,
	description,
	helperText,
	required,
	error,
	value = "",
}) => {
	const form = useFormik({
		initialValues: { value },
		onSubmit: () => {},
	});

	return (
		<FormField
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
		/>
	);
};

const meta: Meta<typeof ExampleFormField> = {
	title: "components/FormField",
	component: ExampleFormField,
	args: {
		id: "story-field",
		label: "Provider name",
	},
};

export default meta;
type Story = StoryObj<typeof ExampleFormField>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(input).not.toHaveAttribute("aria-describedby");
		await expect(input).not.toHaveAttribute("aria-invalid", "true");
		await expect(canvas.queryByText("*")).not.toBeInTheDocument();
	},
};

export const Required: Story = {
	args: {
		required: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("*")).toBeVisible();
	},
};

export const WithDescription: Story = {
	args: {
		description: "Shown to users when selecting this provider.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(input).toHaveAttribute(
			"aria-describedby",
			"story-field-description",
		);
		const description = canvas.getByText(
			"Shown to users when selecting this provider.",
		);
		await expect(description).toHaveAttribute("id", "story-field-description");
	},
};

export const WithHelperText: Story = {
	args: {
		helperText: "Lowercase letters and dashes only.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(input).toHaveAttribute(
			"aria-describedby",
			"story-field-helper",
		);
	},
};

export const WithError: Story = {
	args: {
		error: "Provider name is required.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(input).toHaveAttribute(
			"aria-describedby",
			"story-field-error",
		);
		await expect(input).toHaveAttribute("aria-invalid", "true");
		await expect(canvas.getByText("Provider name is required.")).toBeVisible();
	},
};

export const WithDescriptionAndError: Story = {
	args: {
		description: "Shown to users when selecting this provider.",
		error: "Provider name is required.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(input).toHaveAttribute(
			"aria-describedby",
			"story-field-description story-field-error",
		);
		await expect(input).toHaveAttribute("aria-invalid", "true");
	},
};

export const RequiredWithDescription: Story = {
	args: {
		required: true,
		description: "Shown to users when selecting this provider.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /Provider name/ });
		await expect(canvas.getByText("*")).toBeVisible();
		await expect(input).toHaveAttribute(
			"aria-describedby",
			"story-field-description",
		);
	},
};
