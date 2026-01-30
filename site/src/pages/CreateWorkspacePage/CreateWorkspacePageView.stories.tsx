import { chromatic } from "testHelpers/chromatic";
import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DetailedError } from "api/errors";
import type { PreviewParameter } from "api/typesGenerated";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";

const meta: Meta<typeof CreateWorkspacePageView> = {
	title: "Pages/CreateWorkspacePageView",
	parameters: { chromatic },
	component: CreateWorkspacePageView,
	args: {
		autofillParameters: [],
		diagnostics: [],
		defaultName: "",
		defaultOwner: MockUserOwner,
		externalAuth: [],
		externalAuthPollingState: "idle",
		hasAllRequiredExternalAuth: true,
		mode: "form",
		parameters: [],
		permissions: {
			createWorkspaceForAny: true,
			canUpdateTemplate: false,
		},
		presets: [],
		sendMessage: () => {},
		template: MockTemplate,
	},
};

export default meta;
type Story = StoryObj<typeof CreateWorkspacePageView>;

export const WebsocketError: Story = {
	args: {
		error: new DetailedError(
			"Websocket connection for dynamic parameters unexpectedly closed.",
			"Refresh the page to reset the form.",
		),
	},
};

export const WithViewSourceButton: Story = {
	args: {
		canUpdateTemplate: true,
		versionId: "template-version-123",
		template: {
			...MockTemplate,
			organization_name: "default",
			name: "docker-template",
		},
	},
	parameters: {
		docs: {
			description: {
				story:
					"This story shows the View Source button that appears for template administrators in the experimental workspace creation page. The button allows quick navigation to the template editor.",
			},
		},
	},
};
const parameterInput: PreviewParameter = {
	name: "workspace_name",
	display_name: "Workspace Name",
	description: "A friendly name for your workspace",
	type: "string",
	form_type: "input",
	value: { valid: true, value: "" },
	default_value: { valid: true, value: "my-workspace" },
	diagnostics: [],
	styling: { placeholder: "Enter workspace name" },
	mutable: true,
	icon: "",
	options: [],
	validations: [],
	required: true,
	order: 0,
	ephemeral: false,
};

const parameterDropdown: PreviewParameter = {
	name: "instance_type",
	display_name: "Instance Type",
	description: "The type of cloud instance to provision",
	type: "string",
	form_type: "dropdown",
	value: { valid: true, value: "t3.medium" },
	default_value: { valid: true, value: "t3.medium" },
	diagnostics: [],
	styling: { placeholder: "Select an instance type" },
	mutable: true,
	icon: "/emojis/1f4bb.png",
	options: [
		{
			name: "t3.micro",
			description: "1 vCPU, 1 GB RAM - Free tier eligible",
			value: { valid: true, value: "t3.micro" },
			icon: "/emojis/1f7e2.png",
		},
		{
			name: "t3.small",
			description: "2 vCPU, 2 GB RAM",
			value: { valid: true, value: "t3.small" },
			icon: "/emojis/1f7e1.png",
		},
		{
			name: "t3.medium",
			description: "2 vCPU, 4 GB RAM",
			value: { valid: true, value: "t3.medium" },
			icon: "/emojis/1f7e0.png",
		},
		{
			name: "t3.large",
			description: "2 vCPU, 8 GB RAM",
			value: { valid: true, value: "t3.large" },
			icon: "/emojis/1f534.png",
		},
	],
	validations: [],
	required: true,
	order: 1,
	ephemeral: false,
};

const parameterSlider: PreviewParameter = {
	name: "cpu_count",
	display_name: "CPU Count",
	description: "Number of CPU cores to allocate",
	type: "number",
	form_type: "slider",
	value: { valid: true, value: "4" },
	default_value: { valid: true, value: "2" },
	diagnostics: [],
	styling: {},
	mutable: true,
	icon: "",
	options: [],
	validations: [],
	required: true,
	order: 2,
	ephemeral: false,
};

const parameterSwitch: PreviewParameter = {
	name: "enable_gpu",
	display_name: "Enable GPU",
	description: "Attach a GPU to the workspace for ML workloads",
	type: "bool",
	form_type: "switch",
	value: { valid: true, value: "false" },
	default_value: { valid: true, value: "false" },
	diagnostics: [],
	styling: {},
	mutable: true,
	icon: "",
	options: [],
	validations: [],
	required: false,
	order: 3,
	ephemeral: false,
};

const parameterRadio: PreviewParameter = {
	name: "region",
	display_name: "Region",
	description: "The geographic region for your workspace",
	type: "string",
	form_type: "radio",
	value: { valid: true, value: "us-west-2" },
	default_value: { valid: true, value: "us-west-2" },
	diagnostics: [],
	styling: {},
	mutable: false,
	icon: "",
	options: [
		{
			name: "US West (Oregon)",
			description: "us-west-2",
			value: { valid: true, value: "us-west-2" },
			icon: "/emojis/1f1fa-1f1f8.png",
		},
		{
			name: "US East (N. Virginia)",
			description: "us-east-1",
			value: { valid: true, value: "us-east-1" },
			icon: "/emojis/1f1fa-1f1f8.png",
		},
		{
			name: "EU West (Ireland)",
			description: "eu-west-1",
			value: { valid: true, value: "eu-west-1" },
			icon: "/emojis/1f1ea-1f1fa.png",
		},
	],
	validations: [],
	required: true,
	order: 4,
	ephemeral: false,
};

const parameterMultiSelect: PreviewParameter = {
	name: "ides",
	display_name: "IDEs",
	description: "Select which IDEs to pre-install",
	type: "list(string)",
	form_type: "multi-select",
	value: { valid: true, value: '["vscode", "cursor"]' },
	default_value: { valid: true, value: "[]" },
	diagnostics: [],
	styling: {},
	mutable: true,
	icon: "",
	options: [
		{
			name: "VS Code",
			description: "Visual Studio Code",
			value: { valid: true, value: "vscode" },
			icon: "/icon/code.svg",
		},
		{
			name: "Cursor",
			description: "Cursor IDE",
			value: { valid: true, value: "cursor" },
			icon: "/icon/cursor.svg",
		},
		{
			name: "JetBrains",
			description: "JetBrains IDEs",
			value: { valid: true, value: "jetbrains" },
			icon: "/icon/jetbrains.svg",
		},
		{
			name: "Neovim",
			description: "Neovim editor",
			value: { valid: true, value: "neovim" },
			icon: "/icon/neovim.svg",
		},
	],
	validations: [],
	required: false,
	order: 5,
	ephemeral: false,
};

const parameterTextarea: PreviewParameter = {
	name: "custom_env_vars",
	display_name: "Environment Variables",
	description:
		"Additional environment variables to set in the workspace (one per line, KEY=value format)",
	type: "string",
	form_type: "textarea",
	value: { valid: true, value: "" },
	default_value: { valid: true, value: "" },
	diagnostics: [],
	styling: { placeholder: "NODE_ENV=development\nDEBUG=true" },
	mutable: true,
	icon: "/emojis/1f4dd.png",
	options: [],
	validations: [],
	required: false,
	order: 6,
	ephemeral: false,
};

const parameterCheckbox: PreviewParameter = {
	name: "auto_stop",
	display_name: "Auto-stop",
	description: "Automatically stop workspace after inactivity",
	type: "bool",
	form_type: "checkbox",
	value: { valid: true, value: "true" },
	default_value: { valid: true, value: "true" },
	diagnostics: [],
	styling: {},
	mutable: true,
	icon: "",
	options: [],
	validations: [],
	required: false,
	order: 7,
	ephemeral: false,
};

export const WithParameters: Story = {
	args: {
		parameters: [
			parameterInput,
			parameterDropdown,
			parameterSlider,
			parameterSwitch,
			parameterRadio,
			parameterMultiSelect,
			parameterTextarea,
			parameterCheckbox,
		],
	},
	parameters: {
		docs: {
			description: {
				story:
					"This story demonstrates a workspace creation form with presets and a variety of parameter types including text inputs, dropdowns, sliders, switches, radio buttons, multi-select, textarea, and checkboxes.",
			},
		},
	},
};

export const WithPresets: Story = {
	args: {
		presets: [
			{
				ID: "preset-1",
				Name: "Preset 1",
				Description: "Preset 1 description",
				Parameters: [{ Name: "workspace_name", Value: "my-workspace" }],
				Default: false,
				DesiredPrebuildInstances: null,
				Icon: "/emojis/1f4bb.png",
			},
			{
				ID: "preset-2",
				Name: "Preset 2",
				Description: "Preset 2 description",
				Parameters: [{ Name: "workspace_name", Value: "my-workspace-2" }],
				Default: false,
				DesiredPrebuildInstances: null,
				Icon: "/emojis/1f4bc.png",
			},
		],
		parameters: [parameterInput, parameterDropdown],
	},
};
