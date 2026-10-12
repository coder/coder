import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
	mockGPT5,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

describe("ModelForm", () => {
	it("submits a selected OpenAI reasoning mode", async () => {
		const onUpdateModel = vi.fn(
			async (
				_modelId: string,
				_req: TypesGen.UpdateChatModelRequest,
			): Promise<unknown> => undefined,
		);
		const path = `/ai/settings/models/${mockGPT5.id}`;
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path,
						element: (
							<OrganizationModelsContext.Provider
								value={{
									organization: MockDefaultOrganization,
									accessibleOrganizations: [MockDefaultOrganization],
									permissions: MockOrganizationPermissions,
									requestedOrganizationDenied: false,
								}}
							>
								<ModelForm
									editingModel={mockGPT5}
									providerStates={[
										MockOpenAIProviderState,
										MockAnthropicProviderState,
									]}
									selectedProviderState={MockOpenAIProviderState}
									onProviderChange={vi.fn()}
									isSaving={false}
									isDeleting={false}
									onCreateModel={vi.fn(async () => undefined)}
									onUpdateModel={onUpdateModel}
								/>
							</OrganizationModelsContext.Provider>
						),
					},
				],
				{ initialEntries: [path] },
			),
		);
		const user = userEvent.setup();

		await user.click(
			screen.getByRole("button", { name: /provider configuration/i }),
		);
		await user.click(screen.getByRole("combobox", { name: /reasoning mode/i }));
		await user.click(await screen.findByRole("option", { name: "Pro" }));
		await user.click(screen.getByRole("button", { name: /update model/i }));

		await waitFor(() =>
			expect(onUpdateModel).toHaveBeenCalledWith(
				mockGPT5.id,
				expect.objectContaining({
					model_config: {
						provider_options: { openai: { reasoning_mode: "pro" } },
					},
				}),
			),
		);
	});
});
