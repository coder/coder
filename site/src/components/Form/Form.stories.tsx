import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
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
					<div className="flex flex-col gap-2">
						<Label htmlFor="form-story-workspace-name">Workspace Name</Label>
						<Input id="form-story-workspace-name" name="workspaceName" />
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="form-story-owner">Owner</Label>
						<Input id="form-story-owner" name="owner" />
					</div>
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
