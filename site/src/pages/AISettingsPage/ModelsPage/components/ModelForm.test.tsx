import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
} from "../testFixtures";
import { ModelForm } from "./ModelForm";

describe("ModelForm", () => {
	it("submits a selected OpenAI reasoning mode", async () => {
		const onUpdateModel = vi.fn(async () => undefined);
		const path = `/ai/settings/models/${MockChatModel.id}`;
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
									editingModel={MockChatModel}
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
					{ path: "/ai/settings/models", element: <div>Models</div> },
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
				MockChatModel.id,
				expect.objectContaining({
					model_config: {
						provider_options: { openai: { reasoning_mode: "pro" } },
					},
				}),
			),
		);
	});
});
