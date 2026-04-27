import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { FormFooter } from "#/components/Form/Form";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
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
				<div className="flex flex-col gap-2">
					<Label htmlFor="full-page-form-story-field-1">Field 1</Label>
					<Input id="full-page-form-story-field-1" name="field1" />
				</div>
				<div className="flex flex-col gap-2">
					<Label htmlFor="full-page-form-story-field-2">Field 2</Label>
					<Input id="full-page-form-story-field-2" name="field2" />
				</div>
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
