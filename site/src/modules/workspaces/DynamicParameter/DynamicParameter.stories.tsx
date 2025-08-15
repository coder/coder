import { MockPreviewParameter } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DynamicParameter } from "./DynamicParameter";

const meta: Meta<typeof DynamicParameter> = {
	title: "modules/workspaces/DynamicParameter",
	component: DynamicParameter,
	parameters: {
		layout: "centered",
	},
	args: {
		parameter: MockPreviewParameter,
		onChange: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof DynamicParameter>;

export const TextInput: Story = {};

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
					name: "Nissa, Worldsoul Speaker",
					value: { valid: true, value: "nissa" },
					description:
						"Zendikar still seems so far off, but Chandra is my home.",
					icon: "/emojis/1f7e2.png",
				},
				{
					name: "Canopy Spider",
					value: { valid: true, value: "spider" },
					description:
						"It keeps the upper reaches of the forest free of every menace . . . except for the spider itself.",
					icon: "/emojis/1f7e2.png",
				},
				{
					name: "Ajani, Nacatl Pariah",
					value: { valid: true, value: "ajani" },
					description: "His pride denied him; his brother did not.",
					icon: "/emojis/26aa.png",
				},
				{
					name: "Glowing Anemone",
					value: { valid: true, value: "anemone" },
					description: "Beautiful to behold, terrible to be held.",
					icon: "/emojis/1f535.png",
				},
				{
					name: "Springmantle Cleric",
					value: { valid: true, value: "cleric" },
					description: "Hope and courage bloom in her wake.",
					icon: "/emojis/1f7e2.png",
				},
				{
					name: "Aegar, the Freezing Flame",
					value: { valid: true, value: "aegar" },
					description:
						"Though Phyrexian machines could adapt to extremes of heat or cold, they never figured out how to adapt to both at once.",
					icon: "/emojis/1f308.png",
				},
			],
			styling: {
				placeholder: "Select a creature",
			},
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

export const ErrorFormType: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "error",
			type: "string",
			diagnostics: [
				{
					severity: "error",
					summary: "This is an error",
					detail:
						"This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message.",
					extra: { code: "" },
				},
				{
					severity: "error",
					summary: "This is an error",
					detail:
						"This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message. This is a longer error message.",
					extra: { code: "" },
				},
			],
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

export const Ephemeral: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			ephemeral: true,
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

export const MaskedInput: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "input",
			styling: {
				...MockPreviewParameter.styling,
				placeholder: "Tell me a secret",
				mask_input: true,
			},
		},
	},
};

export const MaskedTextArea: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "textarea",
			styling: {
				...MockPreviewParameter.styling,
				mask_input: true,
			},
		},
	},
};
