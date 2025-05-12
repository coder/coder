import type { Meta, StoryObj } from "@storybook/react";
import { MockPreviewParameter } from "testHelpers/entities";
import { DynamicParameter } from "./DynamicParameter";

const meta: Meta<typeof DynamicParameter> = {
	title: "modules/workspaces/DynamicParameter",
	component: DynamicParameter,
	parameters: {
		layout: "centered",
	},
};

export default meta;
type Story = StoryObj<typeof DynamicParameter>;

export const TextInput: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
		},
	},
};

export const TextArea: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "textarea",
		},
	},
};

export const Checkbox: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "checkbox",
			type: "bool",
		},
	},
};

export const Switch: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "switch",
			type: "bool",
		},
	},
};

export const Dropdown: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "dropdown",
			type: "string",
			options: [
				{
					name: "Option 1",
					value: { valid: true, value: "option1" },
					description: "this is option 1",
					icon: "",
				},
				{
					name: "Option 2",
					value: { valid: true, value: "option2" },
					description: "this is option 2",
					icon: "",
				},
				{
					name: "Option 3",
					value: { valid: true, value: "option3" },
					description: "this is option 3",
					icon: "",
				},
			],
		},
	},
};

export const MultiSelect: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "multi-select",
			type: "list(string)",
			options: [
				{
					name: "Red",
					value: { valid: true, value: "red" },
					description: "this is red",
					icon: "",
				},
				{
					name: "Green",
					value: { valid: true, value: "green" },
					description: "this is green",
					icon: "",
				},
				{
					name: "Blue",
					value: { valid: true, value: "blue" },
					description: "this is blue",
					icon: "",
				},
				{
					name: "Purple",
					value: { valid: true, value: "purple" },
					description: "this is purple",
					icon: "",
				},
			],
		},
	},
};

export const Radio: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "radio",
			type: "string",
			options: [
				{
					name: "Small",
					value: { valid: true, value: "small" },
					description: "this is small",
					icon: "",
				},
				{
					name: "Medium",
					value: { valid: true, value: "medium" },
					description: "this is medium",
					icon: "",
				},
				{
					name: "Large",
					value: { valid: true, value: "large" },
					description: "this is large",
					icon: "",
				},
			],
		},
	},
};

export const Slider: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "slider",
			type: "number",
		},
	},
};

export const Disabled: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			value: { valid: true, value: "disabled value" },
		},
		disabled: true,
	},
};

export const Preset: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			value: { valid: true, value: "preset value" },
		},
		isPreset: true,
	},
};

export const Immutable: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			mutable: false,
		},
	},
};

export const AllBadges: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			value: { valid: true, value: "us-west-2" },
			mutable: false,
		},
		isPreset: true,
	},
};
