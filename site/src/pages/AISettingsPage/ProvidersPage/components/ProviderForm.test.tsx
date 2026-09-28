import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockAIProviderClaudePlatformAWSAPIKey } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import {
	ProviderForm,
	type ProviderFormValues,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";
import {
	aiProviderToFormValues,
	providerFormValuesToUpdate,
} from "./providerFormApiMap";

describe("Claude Platform provider form", () => {
	it.each(["", "workspace-key"])(
		"submits with optional key %j and no auth mode",
		async (key) => {
			const user = userEvent.setup();
			const onSubmit = vi.fn<(values: ProviderFormValues) => void>();
			render(<ProviderForm onSubmit={onSubmit} />);
			await user.click(screen.getByRole("combobox", { name: "Platform" }));
			await user.click(
				screen.getByRole("option", { name: /claude platform for aws/i }),
			);
			await user.type(
				screen.getByRole("textbox", { name: /workspace id/i }),
				"wrkspc_test",
			);
			if (key) {
				await user.type(screen.getByLabelText(/^workspace api key/i), key);
			}
			await user.click(screen.getByRole("button", { name: "Add provider" }));
			await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
			expect(onSubmit.mock.calls[0][0]).toEqual(
				expect.objectContaining({
					authMethod: "claude_platform_aws",
					claudePlatformRegion: "us-east-1",
					claudePlatformWorkspaceId: "wrkspc_test",
					apiKey: key,
				}),
			);
			expect(onSubmit.mock.calls[0][0]).not.toHaveProperty(
				"claudePlatformAuthMode",
			);
		},
	);

	it.each([
		{
			name: "an API-supplied mask",
			savedApiKeyMask:
				MockAIProviderClaudePlatformAWSAPIKey.api_keys[0]?.masked,
			expectedApiKey: MockAIProviderClaudePlatformAWSAPIKey.api_keys[0]?.masked,
		},
		{
			name: "the generic mask when no API mask is supplied",
			savedApiKeyMask: undefined,
			expectedApiKey: SAVED_CREDENTIAL_MASK,
		},
	])(
		"preserves a saved key after focus and blur with $name during an unrelated edit",
		async ({ savedApiKeyMask, expectedApiKey }) => {
			const user = userEvent.setup();
			const provider = MockAIProviderClaudePlatformAWSAPIKey;
			const onSubmit = vi.fn<(values: ProviderFormValues) => void>();
			render(
				<ProviderForm
					editing
					hasSavedApiKey
					savedApiKeyMask={savedApiKeyMask}
					initialValues={aiProviderToFormValues(provider)}
					onSubmit={onSubmit}
				/>,
			);
			await user.click(screen.getByLabelText(/^workspace api key/i));
			await user.click(screen.getByRole("textbox", { name: "Display name" }));
			await user.click(screen.getByRole("combobox", { name: "Platform" }));
			await user.keyboard("{ArrowUp}{Enter}");
			await user.type(
				screen.getByRole("textbox", { name: "Display name" }),
				" updated",
			);
			await user.click(screen.getByRole("button", { name: "Update provider" }));
			await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
			expect(onSubmit.mock.calls[0][0].apiKey).toBe(expectedApiKey);
			const request = providerFormValuesToUpdate(
				onSubmit.mock.calls[0][0],
				provider,
			);
			expect(request.settings).toEqual(provider.settings);
			expect(request.api_keys).toEqual(
				provider.api_keys.map(({ id }) => ({ id })),
			);
		},
	);

	it("updates regional endpoints but preserves a custom endpoint in the submitted values", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn<(values: ProviderFormValues) => void>();
		render(
			<ProviderForm
				initialValues={{
					authMethod: "claude_platform_aws",
					claudePlatformWorkspaceId: "wrkspc_test",
					baseUrl: "https://aws-external-anthropic.us-east-1.api.aws",
				}}
				onSubmit={onSubmit}
			/>,
		);
		const region = screen.getByRole("textbox", { name: /^Region/ });
		await user.clear(region);
		await user.type(region, "_invalid");
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		expect(onSubmit).not.toHaveBeenCalled();
		await user.clear(region);
		await user.type(region, "eu-west-1");
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0]).toEqual(
			expect.objectContaining({
				baseUrl: "https://aws-external-anthropic.eu-west-1.api.aws",
			}),
		);
		const endpoint = screen.getByRole("textbox", { name: /^Endpoint/ });
		await user.clear(endpoint);
		await user.type(endpoint, "https://proxy.example.com/anthropic");
		await user.clear(region);
		await user.type(region, "us-west-2");
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
		expect(onSubmit.mock.calls[1][0]).toEqual(
			expect.objectContaining({
				baseUrl: "https://proxy.example.com/anthropic",
				claudePlatformRegion: "us-west-2",
			}),
		);
	});

	it("requires workspace and region before submission", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn<(values: ProviderFormValues) => void>();
		render(
			<ProviderForm
				initialValues={{ authMethod: "claude_platform_aws" }}
				onSubmit={onSubmit}
			/>,
		);
		await user.clear(screen.getByRole("textbox", { name: /^Region/ }));
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		expect(onSubmit).not.toHaveBeenCalled();
		await user.type(
			screen.getByRole("textbox", { name: /^Region/ }),
			"us-east-1",
		);
		await user.type(
			screen.getByRole("textbox", { name: /workspace id/i }),
			"wrkspc_test",
		);
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
	});
});

it.each([
	{ type: "anthropic", baseUrl: "https://api.anthropic.com", key: "" },
	{
		type: "openai",
		baseUrl: "https://api.openai.com/v1/",
		key: "provider-key",
	},
	{
		type: "azure",
		baseUrl: "https://azure.example.com/openai/v1",
		key: "provider-key",
	},
	{
		type: "google",
		baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
		key: "provider-key",
	},
	{
		type: "openai-compat",
		baseUrl: "https://provider.example.com/v1",
		key: "provider-key",
	},
	{
		type: "openrouter",
		baseUrl: "https://openrouter.ai/api/v1",
		key: "provider-key",
	},
	{
		type: "vercel",
		baseUrl: "https://ai-gateway.vercel.sh/v1",
		key: "provider-key",
	},
	{
		type: "copilot",
		baseUrl: "https://api.business.githubcopilot.com",
		key: "",
	},
] as const)(
	"only relaxes key requirements for $type as appropriate",
	async ({ type, baseUrl, key }) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn<(values: ProviderFormValues) => void>();
		render(
			<ProviderForm initialValues={{ type, baseUrl }} onSubmit={onSubmit} />,
		);
		await user.click(screen.getByRole("button", { name: "Add provider" }));
		if (key) {
			expect(onSubmit).not.toHaveBeenCalled();
			await user.type(screen.getByLabelText(/^API key/i), key);
			await user.click(screen.getByRole("button", { name: "Add provider" }));
		}
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0]).toEqual(
			expect.objectContaining({ type, baseUrl, apiKey: key }),
		);
	},
);
