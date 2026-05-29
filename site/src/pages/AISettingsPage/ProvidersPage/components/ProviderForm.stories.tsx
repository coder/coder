import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { ProviderForm, SAVED_CREDENTIAL_MASK } from "./ProviderForm";

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

type Deferred = {
	promise: Promise<void>;
	resolve: () => void;
};

const createDeferred = (): Deferred => {
	let resolve: (() => void) | undefined;
	const promise = new Promise<void>((res) => {
		resolve = res;
	});
	if (resolve === undefined) {
		throw new Error("deferred resolver was not initialized");
	}
	return { promise, resolve };
};

const SuccessfulSubmitProviderForm = ({
	args,
	deferred,
}: {
	args: ComponentProps<typeof ProviderForm>;
	deferred: Deferred;
}) => {
	const [isLoading, setIsLoading] = useState(false);

	return (
		<ProviderForm
			{...args}
			isLoading={isLoading}
			onSubmit={async (values) => {
				args.onSubmit?.(values);
				setIsLoading(true);
				await deferred.promise;
				setIsLoading(false);
			}}
		/>
	);
};

let bedrockSubmitDeferred = createDeferred();
let apiKeySubmitDeferred = createDeferred();

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
	render: (args) => {
		bedrockSubmitDeferred = createDeferred();
		return (
			<SuccessfulSubmitProviderForm
				args={args}
				deferred={bedrockSubmitDeferred}
			/>
		);
	},
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
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const accessKeyInput = await canvas.findByLabelText(/^access key$/i);
		const accessKeySecretInput =
			await canvas.findByLabelText(/access key secret/i);

		expect(accessKeyInput).toHaveProperty("type", "text");
		expect(accessKeySecretInput).toHaveProperty("type", "text");
		expect(accessKeyInput).toHaveValue(SAVED_CREDENTIAL_MASK);
		expect(accessKeySecretInput).toHaveValue(SAVED_CREDENTIAL_MASK);

		await userEvent.click(accessKeyInput);
		await waitFor(() => expect(accessKeyInput).toHaveValue(""));
		await userEvent.click(accessKeySecretInput);
		await waitFor(() =>
			expect(accessKeyInput).toHaveValue(SAVED_CREDENTIAL_MASK),
		);

		await userEvent.click(accessKeyInput);
		await waitFor(() => expect(accessKeyInput).toHaveValue(""));
		await userEvent.type(accessKeyInput, "AKIAI1lO0EXAMPLE");
		expect(accessKeyInput).toHaveValue("AKIAI1lO0EXAMPLE");

		await userEvent.click(accessKeySecretInput);
		await userEvent.type(accessKeySecretInput, "wJalrI1lO0Secret");
		expect(accessKeySecretInput).toHaveValue("wJalrI1lO0Secret");

		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.clear(displayName);
		await userEvent.type(displayName, "Updated Bedrock");

		const submitButton = canvas.getByRole("button", {
			name: /update provider/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
		await userEvent.click(submitButton);

		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					accessKey: "AKIAI1lO0EXAMPLE",
					accessKeySecret: "wJalrI1lO0Secret",
				}),
			),
		);
		await waitFor(() => expect(submitButton).toBeDisabled());
		bedrockSubmitDeferred.resolve();
		await waitFor(() => {
			expect(accessKeyInput).toHaveValue(SAVED_CREDENTIAL_MASK);
			expect(accessKeySecretInput).toHaveValue(SAVED_CREDENTIAL_MASK);
		});
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
	render: (args) => {
		apiKeySubmitDeferred = createDeferred();
		return (
			<SuccessfulSubmitProviderForm
				args={args}
				deferred={apiKeySubmitDeferred}
			/>
		);
	},
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
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const apiKeyInput = await canvas.findByLabelText(/api key/i);

		expect(apiKeyInput).toHaveProperty("type", "text");
		expect(apiKeyInput).toHaveValue("sk-ant-***\u2026***ABCD");

		await userEvent.click(apiKeyInput);
		await waitFor(() => expect(apiKeyInput).toHaveValue(""));

		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.click(displayName);
		await waitFor(() =>
			expect(apiKeyInput).toHaveValue("sk-ant-***\u2026***ABCD"),
		);

		await userEvent.click(apiKeyInput);
		await waitFor(() => expect(apiKeyInput).toHaveValue(""));
		await userEvent.type(apiKeyInput, "sk-ant-I1lO0-new-secret");
		expect(apiKeyInput).toHaveValue("sk-ant-I1lO0-new-secret");

		await userEvent.clear(displayName);
		await userEvent.type(displayName, "Updated Anthropic");

		const submitButton = canvas.getByRole("button", {
			name: /update provider/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
		await userEvent.click(submitButton);

		await waitFor(() =>
			expect(args.onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					apiKey: "sk-ant-I1lO0-new-secret",
				}),
			),
		);
		await waitFor(() => expect(submitButton).toBeDisabled());
		apiKeySubmitDeferred.resolve();
		await waitFor(() =>
			expect(apiKeyInput).toHaveValue("sk-ant-***\u2026***ABCD"),
		);
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
