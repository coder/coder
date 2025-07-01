import TextField from "@mui/material/TextField";
import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "components/Button/Button";
import { FormFooter } from "components/Form/Form";
import type { FC } from "react";
import { Stack } from "../Stack/Stack";
import { FullPageForm, type FullPageFormProps } from "./FullPageForm";

const Template: FC<FullPageFormProps> = (props) => (
	<FullPageForm {...props}>
		<form
			onSubmit={(e) => {
				e.preventDefault();
			}}
		>
			<Stack>
				<TextField fullWidth label="Field 1" name="field1" />
				<TextField fullWidth label="Field 2" name="field2" />
				<FormFooter>
					<Button variant="outline">Cancel</Button>
					<Button type="submit">Save</Button>
				</FormFooter>
			</Stack>
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
