import TextField from "@mui/material/TextField";
import type { Meta, StoryObj } from "@storybook/react";
import { Form, FormFields, FormSection } from "./Form";

const meta: Meta<typeof Form> = {
	title: "components/Form",
	component: Form,
	args: {
		children: (
			<FormSection
				title="General"
				description="The name of the workspace and its owner. Only admins can create workspaces for other users."
			>
				<FormFields>
					<TextField label="Workspace Name" />
					<TextField label="Owner" />
				</FormFields>
			</FormSection>
		),
	},
};

export default meta;
type Story = StoryObj<typeof Form>;

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
