import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { FormHelpers } from "#/utils/formUtils";
import { ModuleConfiguration } from "./ModuleConfiguration";

const meta: Meta<typeof ModuleConfiguration> = {
	title: "pages/TemplateBuilder/ModuleConfiguration",
	component: ModuleConfiguration,
	args: {
		onRemove: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ModuleConfiguration>;

const stubField = (name: string, value = ""): FormHelpers => ({
	name,
	id: name,
	value,
	onChange: fn(),
	onBlur: fn(),
	error: false,
});

export const Default: Story = {
	args: {
		name: "Claude Code",
		description: "Run the Claude Code agent in your workspace.",
		iconUrl: "/icon/claude.svg",
		detailsUrl: "https://registry.coder.com/modules/claude-code",
		fields: [
			{
				type: "text",
				id: "anthropic-api-key",
				label: "Anthropic API key",
				required: true,
				placeholder: "Enter API key",
				field: stubField("anthropic-api-key"),
			},
			{
				type: "radio",
				id: "other-example",
				label: "Other example",
				required: true,
				options: [
					{ value: "opt-1", label: "Radio text", iconUrl: "/icon/aws.svg" },
					{ value: "opt-2", label: "Radio text", iconUrl: "/icon/aws.svg" },
					{ value: "opt-3", label: "Radio text", iconUrl: "/icon/aws.svg" },
					{ value: "opt-4", label: "Radio text", iconUrl: "/icon/aws.svg" },
				],
			},
			{
				type: "switch-group",
				id: "switch-example",
				label: "Other example",
				required: true,
				switches: [
					{ id: "switch-1", label: "Explaining text", defaultChecked: true },
					{ id: "switch-2", label: "Explaining text", defaultChecked: false },
				],
			},
		],
		optionalFields: [
			{
				type: "select",
				id: "one-more-example",
				label: "One more example",
				options: [
					{ value: "a", label: "Option A" },
					{ value: "b", label: "Option B" },
					{ value: "c", label: "Option C" },
				],
			},
		],
	},
};

export const NoConfiguration: Story = {
	args: {
		name: "Git Clone",
		description: "Clone a Git repository into your workspace on start.",
		iconUrl: "/icon/git.svg",
		detailsUrl: "https://registry.coder.com/modules/git-clone",
	},
};

export const WithoutDetailsLink: Story = {
	args: {
		name: "Custom Module",
		description: "A module without an external details link.",
		iconUrl: "/icon/code.svg",
		fields: [
			{
				type: "text",
				id: "custom-input",
				label: "Configuration value",
				required: true,
				placeholder: "Enter value",
				field: stubField("custom-input"),
			},
		],
	},
};

export const WithoutIcon: Story = {
	args: {
		name: "Unnamed Module",
		description: "A module without an icon.",
		detailsUrl: "https://registry.coder.com",
		fields: [
			{ type: "switch", id: "enabled", label: "Enabled", defaultChecked: true },
		],
	},
};

export const WithSensitiveVariables: Story = {
	args: {
		name: "Claude Code",
		description: "Run the Claude Code agent in your workspace.",
		iconUrl: "/icon/claude.svg",
		detailsUrl: "https://registry.coder.com/modules/claude-code",
		optionalFields: [
			{
				type: "select",
				id: "model",
				label: "Model",
				options: [
					{ value: "sonnet", label: "Sonnet" },
					{ value: "opus", label: "Opus" },
				],
			},
		],
		sensitiveVariables: [
			{
				name: "claude_code_oauth_token",
				type: "string",
				description: "OAuth token used by Claude Code",
				required: true,
				sensitive: true,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const note = await canvas.findByTestId("module-sensitive-variables");
		await expect(note).toBeVisible();
		await expect(note).toHaveTextContent("claude_code_oauth_token");
	},
};

export const NoConfigWithSensitiveVariables: Story = {
	args: {
		name: "OpenAI Codex",
		description: "Install the OpenAI Codex CLI in your workspace.",
		iconUrl: "/icon/openai.svg",
		detailsUrl: "https://registry.coder.com/modules/codex",
		sensitiveVariables: [
			{
				name: "openai_api_key",
				type: "string",
				description: "OpenAI API key",
				required: true,
				sensitive: true,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The sensitive-vars note renders even when the module has no
		// configurable fields (the "No configuration required." branch).
		await expect(canvas.getByText("No configuration required.")).toBeVisible();
		const note = await canvas.findByTestId("module-sensitive-variables");
		await expect(note).toHaveTextContent("openai_api_key");
	},
};
