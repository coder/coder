import type { Meta, StoryObj } from "@storybook/react-vite";
import { TextareaField } from "./TextareaField";

const meta: Meta<typeof TextareaField> = {
	title: "components/TextareaField",
	component: TextareaField,
	args: {},
	argTypes: {
		label: {
			control: "text",
			description: "Label rendered above the textarea",
		},
		error: {
			control: "boolean",
			description: "When true, applies destructive styling to the field",
		},
		helperText: {
			control: "text",
			description: "Text rendered below the textarea",
		},
		fullWidth: {
			control: "boolean",
			description: "When true, the field spans the full width of its container",
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
type Story = StoryObj<typeof TextareaField>;

export const WithLabel: Story = {
	args: {
		label: "Description",
		placeholder: "Enter a description...",
	},
};

export const WithHelperText: Story = {
	args: {
		label: "Message",
		helperText: "Markdown bold, italics, and links are supported.",
		placeholder: "Enter your message...",
	},
};

export const WithError: Story = {
	args: {
		label: "Description",
		error: true,
		helperText: "Description must be at most 128 characters.",
		defaultValue:
			"This value is too long and has triggered a validation error.",
	},
};

export const FullWidth: Story = {
	args: {
		label: "Description",
		helperText: "A short description of your template.",
		placeholder: "Enter a description...",
		fullWidth: true,
		rows: 2,
	},
};

export const Disabled: Story = {
	args: {
		label: "Message",
		placeholder: "Enter a message...",
		disabled: true,
	},
};

export const FormIntegration: Story = {
	args: {
		// Simulates props injected by getFormHelpers()
		name: "description",
		id: "description",
		value: "My workspace template",
		error: false,
		helperText: undefined,
		label: "Description",
		rows: 5,
		fullWidth: true,
	},
};
