import TextField from "@mui/material/TextField";
import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { Form } from "components/Form/Form";
import { useFormik } from "formik";
import type { FC } from "react";
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
		<Form>
			<TextField
				label="Value"
				rows={2}
				{...getFieldHelpers("value", { maxLength })}
			/>
		</Form>
	);
};

const meta: Meta<typeof ExampleForm> = {
	title: "utilities/getFormHelpers",
	component: ExampleForm,
};

export default meta;
type Story = StoryObj<typeof Form>;

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
