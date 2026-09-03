import type { Meta, StoryObj } from "@storybook/react-vite";
import { useFormik } from "formik";
import type { FC } from "react";
import { FormField } from "#/components/FormField/FormField";
import { getFormHelpers } from "#/utils/formUtils";
import { Form, FormFields, FormSection } from "./Form";

const ExampleForm: FC<{ direction?: "horizontal" | "vertical" }> = ({
	direction,
}) => {
	const form = useFormik({
		initialValues: {
			workspaceName: "",
			owner: "",
		},
		onSubmit: () => {},
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<Form direction={direction} onSubmit={form.handleSubmit}>
			<FormSection
				title="General"
				description="The name of the workspace and its owner. Only admins can create workspaces for other users."
			>
				<FormFields>
					<FormField
						label="Workspace Name"
						field={getFieldHelpers("workspaceName")}
					/>
					<FormField label="Owner" field={getFieldHelpers("owner")} />
				</FormFields>
			</FormSection>
		</Form>
	);
};

const meta: Meta<typeof ExampleForm> = {
	title: "components/Form",
	component: ExampleForm,
};

export default meta;
type Story = StoryObj<typeof ExampleForm>;

export const Vertical: Story = {
	args: {
		direction: "vertical",
	},
};

export const Horizontal: Story = {
	args: {
		direction: "horizontal",
	},
};
