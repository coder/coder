import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
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

const SuccessfulSubmitProviderForm = ({
	args,
	deferred,
}: {
	args: ComponentProps<typeof ProviderForm>;
	deferred: Deferred<void>;
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

const FailedSubmitProviderForm = ({
	args,
	deferred,
}: {
	args: ComponentProps<typeof ProviderForm>;
	deferred: Deferred<void>;
}) => {
	const [isLoading, setIsLoading] = useState(false);
	const [submitError, setSubmitError] = useState<unknown>();

	return (
		<ProviderForm
			{...args}
			isLoading={isLoading}
			submitError={submitError}
			onSubmit={async (values) => {
				args.onSubmit?.(values);
				setIsLoading(true);
				await deferred.promise;
				setSubmitError(new Error(errorSubmitMessage));
				setIsLoading(false);
			}}
		/>
	);
};

const ExternalLoadingProviderForm = ({
	args,
	deferred,
}: {
	args: ComponentProps<typeof ProviderForm>;
	deferred: Deferred<void>;
}) => {
	const [isLoading, setIsLoading] = useState(false);

	return (
		<>
			<ProviderForm {...args} isLoading={isLoading} />
			<button
				type="button"
				onClick={async () => {
					setIsLoading(true);
					await deferred.promise;
					setIsLoading(false);
				}}
			>
				Simulate external save
			</button>
		</>
	);
};

const errorSubmitMessage = "Failed to update provider.";

let bedrockSubmitDeferred = createDeferred<void>();
let apiKeySubmitDeferred = createDeferred<void>();
let failedSubmitDeferred = createDeferred<void>();
let externalSaveDeferred = createDeferred<void>();

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

// Mantle is a passthrough protocol selected via the Protocol dropdown. It
// does not configure model fields (the client sends the model), and the
// endpoint hint points at the mantle host.
export const AddBedrockMantle: Story = {
	args: {
		initialValues: { type: "bedrock", protocol: "mantle" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByLabelText(/^model\s*\*?$/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByLabelText(/^small-fast model\s*\*?$/i),
		).not.toBeInTheDocument();
		await expect(canvas.findByText(/bedrock-mantle/i)).resolves.toBeVisible();
	},
};

// Switching the Protocol selector from InvokeModel to Mantle hides the model
// fields and swaps the endpoint hint to the mantle host.
export const AddBedrockSwitchToMantle: Story = {
	args: {
		initialValues: { type: "bedrock" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Model fields are present for the default InvokeModel protocol.
		await canvas.findByLabelText(/^model\s*\*?$/i);

		const trigger = canvas.getByRole("combobox", { name: /protocol/i });
		await userEvent.click(trigger);
		const mantleOption = await screen.findByRole("option", {
			name: /mantle/i,
		});
		await userEvent.click(mantleOption);

		await waitFor(() =>
			expect(canvas.queryByLabelText(/^model\s*\*?$/i)).not.toBeInTheDocument(),
		);
		expect(
			canvas.queryByLabelText(/^small-fast model\s*\*?$/i),
		).not.toBeInTheDocument();
		await expect(canvas.findByText(/bedrock-mantle/i)).resolves.toBeVisible();
	},
};

// Switching protocol must not leave a stale endpoint validation error that
// keeps Save disabled. Protocol and base URL are updated atomically, so after
// switching a fully valid config keeps the submit button enabled.
export const AddBedrockProtocolSwitchKeepsSaveEnabled: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock",
			baseUrl: "https://bedrock-runtime.eu-west-1.amazonaws.com",
			model: "anthropic.claude-sonnet-4-5",
			smallFastModel: "anthropic.claude-haiku-4-5",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const submit = canvas.getByRole("button", { name: /add provider/i });

		// Switch InvokeModel -> Mantle.
		await userEvent.click(canvas.getByRole("combobox", { name: /protocol/i }));
		await userEvent.click(
			await screen.findByRole("option", { name: /mantle/i }),
		);

		// The endpoint is rewritten to a valid mantle URL and the model fields
		// drop out, so the form stays valid and Save is not disabled by a stale
		// endpoint error.
		await waitFor(() => {
			expect(canvas.queryByLabelText(/^model\s*\*?$/i)).not.toBeInTheDocument();
			expect(canvas.getByLabelText(/^endpoint\s*\*?$/i)).toHaveValue(
				"https://bedrock-mantle.eu-west-1.api.aws/anthropic",
			);
			expect(submit).toBeEnabled();
		});
	},
};

// Reverse switch: Mantle -> InvokeModel restores the model fields and rewrites
// the endpoint back to the InvokeModel host, preserving the region the user
// already entered.
export const AddBedrockSwitchToInvokeModel: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock",
			protocol: "mantle",
			baseUrl: "https://bedrock-mantle.eu-west-1.api.aws/anthropic",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Model fields are hidden under Mantle.
		expect(canvas.queryByLabelText(/^model\s*\*?$/i)).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("combobox", { name: /protocol/i }));
		await userEvent.click(
			await screen.findByRole("option", { name: /invokemodel/i }),
		);

		// The model fields return and the endpoint is rewritten to the
		// InvokeModel host, keeping the eu-west-1 region.
		await canvas.findByLabelText(/^model\s*\*?$/i);
		await canvas.findByLabelText(/^small-fast model\s*\*?$/i);
		await waitFor(() =>
			expect(canvas.getByLabelText(/^endpoint\s*\*?$/i)).toHaveValue(
				"https://bedrock-runtime.eu-west-1.amazonaws.com",
			),
		);
	},
};

// When the current endpoint has no parseable region (e.g. a blank field),
// switching protocol falls back to us-east-1 rather than producing an
// invalid URL.
export const AddBedrockProtocolSwitchRegionFallback: Story = {
	args: {
		initialValues: { type: "bedrock", baseUrl: "" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Model fields are present for the default InvokeModel protocol.
		await canvas.findByLabelText(/^model\s*\*?$/i);

		await userEvent.click(canvas.getByRole("combobox", { name: /protocol/i }));
		await userEvent.click(
			await screen.findByRole("option", { name: /mantle/i }),
		);

		// The blank endpoint has no region, so the rewrite falls back to
		// us-east-1.
		await waitFor(() =>
			expect(canvas.getByLabelText(/^endpoint\s*\*?$/i)).toHaveValue(
				"https://bedrock-mantle.us-east-1.api.aws/anthropic",
			),
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

// Under mantle, the endpoint must match the mantle host. An InvokeModel-shaped
// URL is rejected by the schema, so Save stays disabled and onSubmit never
// fires. Guards the protocol-conditional baseUrl validation.
export const AddBedrockMantleRejectsInvokeUrl: Story = {
	args: {
		initialValues: {
			type: "bedrock",
			name: "bedrock-mantle",
			protocol: "mantle",
			baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
			model: "",
			smallFastModel: "",
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button", { name: /add provider/i });

		await waitFor(() => expect(submitButton).toBeDisabled());
		expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const EditBedrockKeepCredentials: Story = {
	render: (args) => {
		bedrockSubmitDeferred = createDeferred<void>();
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
		const accessKeyInput = await canvas.findByLabelText(/^access key\s*\*?$/i);
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
		await waitFor(() => expect(accessKeySecretInput).toHaveValue(""));
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

// The external ID is server-generated when a role is assumed. The edit form
// surfaces it read-only next to Role ARN so operators can add the value to
// their role's trust policy as an sts:ExternalId condition.
export const EditBedrockWithExternalId: Story = {
	args: {
		editing: true,
		bedrockSavedAccessCredentials: true,
		bedrockExternalId: "7QF3ZK2MLP4RS6TUVWXY2ABCDE",
		initialValues: {
			type: "bedrock",
			name: "bedrock",
			displayName: "Bedrock",
			baseUrl: "https://bedrock-runtime.us-east-2.amazonaws.com",
			model: "anthropic.claude-opus-4-7",
			smallFastModel: "anthropic.claude-haiku-4-5",
			accessKey: "",
			accessKeySecret: "",
			roleArn: "arn:aws:iam::123456789012:role/BedrockRole",
			enabled: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.findByText("External ID")).resolves.toBeVisible();
		await expect(
			canvas.findByText("7QF3ZK2MLP4RS6TUVWXY2ABCDE"),
		).resolves.toBeVisible();
		await expect(canvas.findByText("sts:ExternalId")).resolves.toBeVisible();
		await expect(
			canvas.findByRole("button", { name: /copy code/i }),
		).resolves.toBeVisible();
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
	render: (args) => {
		apiKeySubmitDeferred = createDeferred<void>();
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
export const FailedSubmitKeepsCredential: Story = {
	render: (args) => {
		failedSubmitDeferred = createDeferred<void>();
		return (
			<FailedSubmitProviderForm args={args} deferred={failedSubmitDeferred} />
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

		await userEvent.click(apiKeyInput);
		await waitFor(() => expect(apiKeyInput).toHaveValue(""));
		await userEvent.type(apiKeyInput, "sk-ant-I1lO0-new-secret");

		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.clear(displayName);
		await userEvent.type(displayName, "Failed Anthropic");

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
		failedSubmitDeferred.resolve();
		await expect(await canvas.findByText(errorSubmitMessage)).toBeVisible();
		expect(apiKeyInput).toHaveValue("sk-ant-I1lO0-new-secret");
	},
};

export const ExternalLoadingKeepsCredential: Story = {
	render: (args) => {
		externalSaveDeferred = createDeferred<void>();
		return (
			<ExternalLoadingProviderForm
				args={args}
				deferred={externalSaveDeferred}
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const apiKeyInput = await canvas.findByLabelText(/api key/i);
		const submitButton = canvas.getByRole("button", {
			name: /update provider/i,
		});

		await userEvent.click(apiKeyInput);
		await waitFor(() => expect(apiKeyInput).toHaveValue(""));
		await userEvent.type(apiKeyInput, "sk-ant-I1lO0-new-secret");
		await waitFor(() => expect(submitButton).toBeEnabled());

		await userEvent.click(
			canvas.getByRole("button", { name: /simulate external save/i }),
		);
		await waitFor(() => expect(submitButton).toBeDisabled());
		externalSaveDeferred.resolve();
		await waitFor(() => expect(submitButton).toBeEnabled());
		expect(apiKeyInput).toHaveValue("sk-ant-I1lO0-new-secret");
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
