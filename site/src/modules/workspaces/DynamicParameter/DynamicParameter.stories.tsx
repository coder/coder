import type { Meta, StoryObj } from "@storybook/react";
import { MockPreviewParameter } from "testHelpers/entities";
import { DynamicParameter } from "./DynamicParameter";

const meta: Meta<typeof DynamicParameter> = {
	title: "modules/workspaces/DynamicParameter",
	component: DynamicParameter,
	parameters: {
		layout: "centered",
	},
	args: {
		parameter: MockPreviewParameter,
		onChange: () => { },
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

export const DropdownWithManyOptions: Story = {
	args: {
		parameter: {
			...MockPreviewParameter,
			form_type: "dropdown",
			type: "string",
			options: [
				{ name: "JavaScript", value: { valid: true, value: "javascript" }, description: "JavaScript programming language", icon: "" },
				{ name: "TypeScript", value: { valid: true, value: "typescript" }, description: "TypeScript programming language", icon: "" },
				{ name: "Python", value: { valid: true, value: "python" }, description: "Python programming language", icon: "" },
				{ name: "Java", value: { valid: true, value: "java" }, description: "Java programming language", icon: "" },
				{ name: "C++", value: { valid: true, value: "cpp" }, description: "C++ programming language", icon: "" },
				{ name: "C#", value: { valid: true, value: "csharp" }, description: "C# programming language", icon: "" },
				{ name: "Ruby", value: { valid: true, value: "ruby" }, description: "Ruby programming language", icon: "" },
				{ name: "Go", value: { valid: true, value: "go" }, description: "Go programming language", icon: "" },
				{ name: "Rust", value: { valid: true, value: "rust" }, description: "Rust programming language", icon: "" },
				{ name: "Swift", value: { valid: true, value: "swift" }, description: "Swift programming language", icon: "" },
				{ name: "Kotlin", value: { valid: true, value: "kotlin" }, description: "Kotlin programming language", icon: "" },
				{ name: "Scala", value: { valid: true, value: "scala" }, description: "Scala programming language", icon: "" },
				{ name: "PHP", value: { valid: true, value: "php" }, description: "PHP programming language", icon: "" },
				{ name: "Perl", value: { valid: true, value: "perl" }, description: "Perl programming language", icon: "" },
				{ name: "R", value: { valid: true, value: "r" }, description: "R programming language", icon: "" },
				{ name: "MATLAB", value: { valid: true, value: "matlab" }, description: "MATLAB programming language", icon: "" },
				{ name: "Julia", value: { valid: true, value: "julia" }, description: "Julia programming language", icon: "" },
				{ name: "Dart", value: { valid: true, value: "dart" }, description: "Dart programming language", icon: "" },
				{ name: "Lua", value: { valid: true, value: "lua" }, description: "Lua programming language", icon: "" },
				{ name: "Haskell", value: { valid: true, value: "haskell" }, description: "Haskell programming language", icon: "" },
			],
			styling: {
				placeholder: "Select a programming language",
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
