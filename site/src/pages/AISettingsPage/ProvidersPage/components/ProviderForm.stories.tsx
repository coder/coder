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
		initialValues: { type: "bedrock" },
	},
	play: async ({ canvasElement }) => {
		// AddProviderPageView passes only `{ type }`; verify the form
		// pre-fills the deployment defaults for the model fields so an
		// operator does not have to copy them from the docs.
		const canvas = within(canvasElement);
		const modelInput = await canvas.findByLabelText(/^model\s*\*?$/i);
		const smallFastModelInput = await canvas.findByLabelText(
			/^small-fast model\s*\*?$/i,
		);
		expect(modelInput).toHaveValue(
			"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
		);
		expect(smallFastModelInput).toHaveValue(
			"global.anthropic.claude-haiku-4-5-20251001-v1:0",
		);
	},
};

// Regression coverage for CODAGT-626. The create form must accept Bedrock
// configurations whose credentials come from the AWS environment (IAM
// role, instance profile, AWS_PROFILE) instead of static access keys.
export const AddBedrockWithoutStaticCredentials: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock-iam",
			displayName: "Bedrock IAM",
			baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
			model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
			smallFastModel: "anthropic.claude-3-5-haiku-20241022-v1:0",
			accessKey: "",
			accessKeySecret: "",
			enabled: true,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const accessKeyInput = await canvas.findByLabelText(/^access key\s*$/i);
		const accessKeySecretInput =
			await canvas.findByLabelText(/access key secret/i);

		// Neither field renders the required asterisk.
		expect(accessKeyInput).toHaveValue("");
		expect(accessKeySecretInput).toHaveValue("");

		// The Add provider button is enabled even with both credentials blank.
		const submitButton = canvas.getByRole("button", {
			name: /add provider/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
		await userEvent.click(submitButton);

		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					type: "bedrock",
					accessKey: "",
					accessKeySecret: "",
				}),
			),
		);
	},
};

// A half-typed credential pair is blocked at the form layer because the
// backend treats access_key and access_key_secret as a pair. This story
// keeps the cross-validation honest.
export const AddBedrockHalfCredentialPairBlocked: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock-half",
			displayName: "Bedrock Half",
			baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
			model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
			smallFastModel: "anthropic.claude-3-5-haiku-20241022-v1:0",
			accessKey: "",
			accessKeySecret: "",
			enabled: true,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const accessKeyInput = await canvas.findByLabelText(/^access key\s*$/i);

		await userEvent.type(accessKeyInput, "AKIAIOSFODNN7EXAMPLE");

		const submitButton = canvas.getByRole("button", {
			name: /add provider/i,
		});
		await waitFor(() => expect(submitButton).toBeDisabled());
		expect(args.onSubmit).not.toHaveBeenCalled();
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

export const AddCopilot: Story = {
	args: {
		// The real add flow passes only the type; the form fills name and
		// endpoint from the copilot defaults.
		initialValues: { type: "copilot" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByLabelText(/endpoint/i);
		expect(canvas.queryByLabelText(/api key/i)).not.toBeInTheDocument();
	},
};

export const EditCopilot: Story = {
	args: {
		editing: true,
		initialValues: {
			type: "copilot",
			name: "copilot",
			displayName: "GitHub Copilot",
			baseUrl: "https://api.business.githubcopilot.com",
			enabled: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const name = await canvas.findByLabelText(/^name/i);
		expect(name).toBeDisabled();
		expect(canvas.queryByLabelText(/api key/i)).not.toBeInTheDocument();
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
