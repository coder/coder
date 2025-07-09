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
					name: "Go",
					value: { valid: true, value: "go" },
					description: "Go 1.24, gofumpt, golangci-lint",
					icon: "/icon/go.svg",
				},
				{
					name: "Kotlin/Java",
					value: { valid: true, value: "jvm" },
					description: "OpenJDK 24 and Gradle",
					icon: "/icon/kotlin.svg",
				},
				{
					name: "Rust",
					value: { valid: true, value: "rust" },
					description: "rustup w/ stable and nightly toolchains",
					icon: "/icon/rust.svg",
				},
				{
					name: "TypeScript/JavaScript",
					value: { valid: true, value: "js" },
					description: "Node.js 24, fnm, and npm/yarn/pnpm via corepack",
					icon: "/icon/typescript.svg",
				},

				{
					name: "C++",
					value: { valid: true, value: "cpp" },
					description: "gcc 15.1, CMake, ninja, autotools, vcpkg, clang-format",
					icon: "/icon/cpp.svg",
				},
				{
					name: "C#",
					value: { valid: true, value: "csharp" },
					description: ".NET 9",
					icon: "/icon/dotnet.svg",
				},
				{
					name: "Dart",
					value: { valid: true, value: "dart" },
					description: "Dart 3",
					icon: "https://github.com/dart-lang.png",
				},
				{
					name: "Julia",
					value: { valid: true, value: "julia" },
					description: "Julia 1.10",
					icon: "https://github.com/JuliaLang.png",
				},
				{
					name: "PHP",
					value: { valid: true, value: "php" },
					description: "PHP 8.4",
					icon: "/icon/php.svg",
				},
				{
					name: "Python",
					value: { valid: true, value: "python" },
					description: "Python 3.13 and uv",
					icon: "/icon/python.svg",
				},
				{
					name: "Swift",
					value: { valid: true, value: "swift" },
					description: "Swift 6.1",
					icon: "/icon/swift.svg",
				},
				{
					name: "Ruby",
					value: { valid: true, value: "ruby" },
					description: "Ruby 3.4, bundle, rufo",
					icon: "/icon/ruby.png",
				},
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
