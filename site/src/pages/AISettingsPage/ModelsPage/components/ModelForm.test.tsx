import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { ChatModel } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockGPT56Pro,
	MockOpenAIProviderState,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

const renderModelForm = (editingModel?: ChatModel) => {
	const onCreateModel = vi.fn(async () => undefined);
	const onUpdateModel = vi.fn(async () => undefined);
	const element = (
		<OrganizationModelsContext.Provider
			value={{
				organization: MockDefaultOrganization,
				accessibleOrganizations: [MockDefaultOrganization],
				permissions: MockOrganizationPermissions,
				requestedOrganizationDenied: false,
			}}
		>
			<ModelForm
				editingModel={editingModel}
				providerStates={[MockOpenAIProviderState, MockAnthropicProviderState]}
				selectedProviderState={MockOpenAIProviderState}
				onProviderChange={vi.fn()}
				isSaving={false}
				isDeleting={false}
				onCreateModel={onCreateModel}
				onUpdateModel={onUpdateModel}
			/>
		</OrganizationModelsContext.Provider>
	);
	const path = editingModel
		? `/ai/settings/models/${editingModel.id}`
		: "/ai/settings/models/add";
	renderWithRouter(
		createMemoryRouter(
			[
				{ path, element },
				{ path: "/ai/settings/models", element: <div>Models</div> },
			],
			{ initialEntries: [path] },
		),
	);
	return { onCreateModel, onUpdateModel };
};

const openProviderConfig = (user: ReturnType<typeof userEvent.setup>) =>
	user.click(screen.getByRole("button", { name: /provider configuration/i }));

const selectOption = async (
	user: ReturnType<typeof userEvent.setup>,
	name: RegExp,
	option: string,
) => {
	await user.click(screen.getByRole("combobox", { name }));
	await user.click(await screen.findByRole("option", { name: option }));
};

const proModelConfig = MockGPT56Pro.model_config;

describe("ModelForm reasoning mode", () => {
	it("saves Pro for a new GPT-5.6 model", async () => {
		const user = userEvent.setup();
		const { onCreateModel } = renderModelForm();

		await user.type(screen.getByLabelText(/model identifier/i), "gpt-5.6");
		await user.click(
			await screen.findByRole("option", { name: /GPT-5.6 Sol/i }),
		);
		await user.clear(screen.getByLabelText(/context limit/i));
		await user.type(screen.getByLabelText(/context limit/i), "200000");
		await openProviderConfig(user);
		await selectOption(user, /reasoning mode/i, "Pro");
		await user.click(screen.getByRole("button", { name: /add model/i }));

		await waitFor(() =>
			expect(onCreateModel).toHaveBeenCalledWith(
				expect.objectContaining({
					model: "gpt-5.6-sol",
					model_config: expect.objectContaining({
						provider_options: {
							openai: expect.objectContaining({ reasoning_mode: "pro" }),
						},
					}),
				}),
			),
		);
	});

	it("changes and clears the mode without touching effort or tier", async () => {
		const user = userEvent.setup();
		const { onUpdateModel } = renderModelForm(MockGPT56Pro);

		await openProviderConfig(user);
		await selectOption(user, /reasoning mode/i, "Standard");
		await user.click(screen.getByRole("button", { name: /update model/i }));
		await waitFor(() =>
			expect(onUpdateModel).toHaveBeenLastCalledWith(
				MockGPT56Pro.id,
				expect.objectContaining({
					model_config: {
						...proModelConfig,
						provider_options: {
							openai: { reasoning_mode: "standard", service_tier: "priority" },
						},
					},
				}),
			),
		);

		await selectOption(user, /reasoning mode/i, "Default");
		await user.click(screen.getByRole("button", { name: /update model/i }));
		await waitFor(() =>
			expect(onUpdateModel).toHaveBeenLastCalledWith(
				MockGPT56Pro.id,
				expect.objectContaining({
					model_config: {
						...proModelConfig,
						provider_options: { openai: { service_tier: "priority" } },
					},
				}),
			),
		);
	});

	it("drops a stale mode when the model stops supporting it", async () => {
		const user = userEvent.setup();
		const { onUpdateModel } = renderModelForm(MockGPT56Pro);

		const modelInput = screen.getByLabelText(/model identifier/i);
		await user.clear(modelInput);
		await user.type(modelInput, "gpt-5.6-pro");
		await user.tab();
		await user.click(screen.getByRole("button", { name: /update model/i }));

		await waitFor(() =>
			expect(onUpdateModel).toHaveBeenCalledWith(
				MockGPT56Pro.id,
				expect.objectContaining({
					model: "gpt-5.6-pro",
					model_config: {
						...proModelConfig,
						provider_options: { openai: { service_tier: "priority" } },
					},
				}),
			),
		);
	});
});
