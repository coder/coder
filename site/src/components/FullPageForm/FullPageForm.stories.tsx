import type { Meta, StoryObj } from "@storybook/react-vite";
import { useFormik } from "formik";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { getFormHelpers } from "#/utils/formUtils";
import { FullPageForm, type FullPageFormProps } from "./FullPageForm";

const Template: FC<FullPageFormProps> = (props) => {
	const form = useFormik({
		initialValues: {
			field1: "",
			field2: "",
		},
		onSubmit: () => {},
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<FullPageForm {...props}>
			<form onSubmit={form.handleSubmit}>
				<div className="flex flex-col gap-4">
					<FormField label="Field 1" field={getFieldHelpers("field1")} />
					<FormField label="Field 2" field={getFieldHelpers("field2")} />
					<FormFooter>
						<Button variant="outline">Cancel</Button>
						<Button type="submit">Save</Button>
					</FormFooter>
				</div>
			</form>
		</FullPageForm>
	);
};

const meta: Meta<typeof FullPageForm> = {
	title: "components/FullPageForm",
	component: Template,
};

export default meta;
type Story = StoryObj<typeof FullPageForm>;

const Example: Story = {
	args: {
		title: "My Form",
		detail: "Lorem ipsum dolor",
	},
};

export { Example as FullPageForm };
