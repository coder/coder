import TextField from "@mui/material/TextField";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { FormFooter } from "#/components/Form/Form";
import { FullPageForm, type FullPageFormProps } from "./FullPageForm";

const Template: FC<FullPageFormProps> = (props) => (
	<FullPageForm {...props}>
		<form
			onSubmit={(e) => {
				e.preventDefault();
			}}
		>
			<div className="flex flex-col gap-4">
				<TextField fullWidth label="Field 1" name="field1" />
				<TextField fullWidth label="Field 2" name="field2" />
				<FormFooter>
					<Button variant="outline">Cancel</Button>
					<Button type="submit">Save</Button>
				</FormFooter>
			</div>
		</form>
	</FullPageForm>
);

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
