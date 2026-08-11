import type { Meta, StoryObj } from "@storybook/react-vite";
import { useFormik } from "formik";
import type { FC } from "react";
import { action } from "storybook/actions";
import { FormField } from "#/components/FormField/FormField";
import { getFormHelpers } from "./formUtils";

interface ExampleFormProps {
	value?: string;
	maxLength?: number;
}

const ExampleForm: FC<ExampleFormProps> = ({ value, maxLength }) => {
	const form = useFormik({
		initialValues: {
			value,
		},
		onSubmit: action("submit"),
	});

	const getFieldHelpers = getFormHelpers(form, null);

	return (
		<FormField label="Value" field={getFieldHelpers("value", { maxLength })} />
	);
};

const meta: Meta<typeof ExampleForm> = {
	title: "utilities/getFormHelpers",
	component: ExampleForm,
};

export default meta;
type Story = StoryObj<typeof ExampleForm>;

export const UnderMaxLength: Story = {
	args: {
		value: "a".repeat(98),
		maxLength: 128,
	},
};

export const CloseToMaxLength: Story = {
	args: {
		value: "a".repeat(99),
		maxLength: 128,
	},
};

export const AtMaxLength: Story = {
	args: {
		value: "a".repeat(128),
		maxLength: 128,
	},
};

export const OverMaxLength: Story = {
	args: {
		value: "a".repeat(129),
		maxLength: 128,
	},
};
