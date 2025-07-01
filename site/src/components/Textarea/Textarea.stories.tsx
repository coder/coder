import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import { useState } from "react";
import { Textarea } from "./Textarea";

const meta: Meta<typeof Textarea> = {
	title: "components/Textarea",
	component: Textarea,
	args: {},
	argTypes: {
		value: {
			control: "text",
			description: "The controlled value of the textarea",
		},
		defaultValue: {
			control: "text",
			description: "The default value when initially rendered",
		},
		disabled: {
			control: "boolean",
			description:
				"When true, prevents the user from interacting with the textarea",
		},
		placeholder: {
			control: "text",
			description: "Placeholder text displayed when the textarea is empty",
		},
		rows: {
			control: "number",
			description: "The number of rows to display",
		},
	},
};

export default meta;
type Story = StoryObj<typeof Textarea>;

export const WithPlaceholder: Story = {
	args: {
		placeholder: "Enter your message here...",
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
		placeholder: "Placeholder",
	},
};

export const WithDefaultValue: Story = {
	args: {
		defaultValue: "This is some default text in the textarea.",
	},
};

export const Large: Story = {
	args: {
		rows: 8,
		placeholder: "Placeholder: A larger textarea with more rows",
	},
};

const ControlledTextarea = () => {
	const [value, setValue] = useState("This is a controlled textarea.");
	return (
		<div className="space-y-2">
			<Textarea
				value={value}
				placeholder="Type something..."
				onChange={(e) => setValue(e.target.value)}
			/>
			<div className="text-sm text-content-secondary">
				Character count: {value.length}
			</div>
		</div>
	);
};

export const Controlled: Story = {
	render: () => <ControlledTextarea />,
};

export const TypeText: Story = {
	args: {
		placeholder: "Type something here...",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textarea = canvas.getByRole("textbox");
		await userEvent.type(
			textarea,
			"Hello, this is some example text being typed into the textarea!",
		);
		expect(textarea).toHaveValue(
			"Hello, this is some example text being typed into the textarea!",
		);
	},
};
