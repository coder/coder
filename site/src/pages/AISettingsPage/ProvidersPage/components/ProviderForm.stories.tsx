import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { ProviderForm } from "./ProviderForm";

const meta: Meta<typeof ProviderForm> = {
	title: "pages/AISettingsPage/ProviderForm",
	component: ProviderForm,
	args: {
		editing: false,
		isLoading: false,
		onSubmit: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProviderForm>;

export const AddAnthropicDefault: Story = {};

export const AddOpenAI: Story = {
	args: {
		initialValues: {
			type: "openai",
			name: "corporate-openai",
			displayName: "Corporate OpenAI",
			baseUrl: "https://api.openai.com/v1",
			apiKey: "sk-example",
			enabled: true,
		},
	},
};

export const AddBedrock: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock-prod",
			displayName: "Bedrock Prod",
			baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
			model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
			smallFastModel: "anthropic.claude-3-5-haiku-20241022-v1:0",
			accessKey: "AKIAIOSFODNN7EXAMPLE",
			accessKeySecret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			enabled: true,
		},
	},
};

export const EditBedrockKeepCredentials: Story = {
	args: {
		editing: true,
		bedrockSavedAccessCredentials: true,
		initialValues: {
			type: "bedrock",
			name: "bedrock",
			displayName: "Bedrock",
			baseUrl: "https://bedrock-runtime.us-east-2.amazonaws.com",
			model: "anthropic.claude-opus-4-7",
			smallFastModel: "anthropic.claude-haiku-4-5",
			accessKey: "",
			accessKeySecret: "",
			enabled: true,
		},
	},
};

export const EditProvider: Story = {
	args: {
		editing: true,
		openAiAnthropicSavedApiKey: true,
		openAiAnthropicMaskedApiKey: "sk-ant-***\u2026***ABCD",
		initialValues: {
			type: "anthropic",
			name: "production-anthropic",
			displayName: "Production Anthropic",
			baseUrl: "https://api.anthropic.com",
			apiKey: "",
			enabled: true,
		},
	},
};

export const EditOpenAiAnthropicNoSavedKey: Story = {
	args: {
		editing: true,
		openAiAnthropicSavedApiKey: false,
		initialValues: {
			type: "anthropic",
			name: "production-anthropic",
			displayName: "Production Anthropic",
			baseUrl: "https://api.anthropic.com",
			apiKey: "",
			enabled: true,
		},
	},
};

export const Submitting: Story = {
	args: {
		isLoading: true,
		initialValues: {
			type: "openai",
			name: "openai",
			displayName: "OpenAI",
			baseUrl: "https://api.openai.com/v1",
			apiKey: "sk-example",
		},
	},
};

export const CredentialFocusClear: Story = {
	args: {
		editing: true,
		openAiAnthropicSavedApiKey: true,
		openAiAnthropicMaskedApiKey: "sk-ant-***\u2026***ABCD",
		initialValues: {
			type: "anthropic",
			name: "production-anthropic",
			displayName: "Production Anthropic",
			baseUrl: "https://api.anthropic.com",
			apiKey: "",
			enabled: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const apiKeyInput = await canvas.findByLabelText(/api key/i);
		expect(apiKeyInput).toHaveValue("sk-ant-***\u2026***ABCD");
		await userEvent.click(apiKeyInput);
		await waitFor(() => expect(apiKeyInput).toHaveValue(""));
	},
};

export const UnsavedChangesPrompt: Story = {
	args: {
		editing: true,
		initialValues: {
			type: "openai",
			name: "corporate-openai",
			displayName: "Corporate OpenAI",
			baseUrl: "https://api.openai.com/v1",
			apiKey: "",
			enabled: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Dirty the form by editing the display name.
		const displayName = await canvas.findByLabelText(/display name/i);
		await userEvent.type(displayName, " Edited");
		// Attempt to leave via the in-form Cancel link.
		const cancelLink = canvas.getByRole("link", { name: /cancel/i });
		await userEvent.click(cancelLink);
		// The dialog renders in a portal, so search the document.
		const dialog = await screen.findByRole("dialog");
		await expect(
			within(dialog).getByText("Unsaved changes"),
		).toBeInTheDocument();
		await expect(
			within(dialog).getByText(/your updates haven't been saved/i),
		).toBeInTheDocument();
	},
};
