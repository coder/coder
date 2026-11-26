import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import ModelsPageView from "./ModelsPageView";
import { OrganizationModelsContext } from "./organizationModels";
import {
	MockAnthropicProviderState,
	MockBedrockProviderState,
	MockOpenAIProviderState,
	mockBedrockClaude,
	mockClaude,
	mockDisabledModel,
	mockGPT5,
} from "./testFixtures";

const renderModelsPageView = (
	initialEntry = "/ai/settings/models",
	accessibleOrganizations = [MockDefaultOrganization],
) => {
	const element = (
		<OrganizationModelsContext.Provider
			value={{
				organization: MockDefaultOrganization,
				accessibleOrganizations,
				permissions: MockOrganizationPermissions,
				requestedOrganizationDenied: false,
			}}
		>
			<ModelsPageView
				isLoading={false}
				loadError={null}
				refetchError={null}
				models={[mockGPT5, mockClaude, mockDisabledModel, mockBedrockClaude]}
				providerStates={[
					MockOpenAIProviderState,
					MockAnthropicProviderState,
					MockBedrockProviderState,
				]}
				providerTypeByID={
					new Map([
						["prov-openai", "openai"],
						["prov-anthropic", "anthropic"],
						["prov-bedrock", "bedrock"],
					])
				}
				canCreateModel
			/>
		</OrganizationModelsContext.Provider>
	);

	return renderWithRouter(
		createMemoryRouter(
			[
				{ path: "/ai/settings/models", element },
				{ path: "/ai/settings/models/:modelId", element },
			],
			{ initialEntries: [initialEntry] },
		),
	);
};

describe("ModelsPageView", () => {
	it("names the toolbar filters with their label and value", async () => {
		renderModelsPageView("/ai/settings/models", [
			MockDefaultOrganization,
			MockOrganization2,
		]);

		expect(
			await screen.findByRole("combobox", { name: "Provider: All" }),
		).toBeInTheDocument();
		expect(
			screen.getByRole("combobox", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).toHaveTextContent(/^Organization:/);
	});

	it("filters models by the provider in the URL", async () => {
		renderModelsPageView("/ai/settings/models?provider=prov-anthropic");

		expect(
			await screen.findByRole("button", { name: /Claude Sonnet 4.5/i }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /GPT-5/i }),
		).not.toBeInTheDocument();
	});

	it("keeps the provider filter in the URL when opening a model", async () => {
		const user = userEvent.setup();
		const { router } = renderModelsPageView();

		await user.click(screen.getByRole("combobox", { name: /^Provider:/ }));
		await user.click(await screen.findByRole("option", { name: "Anthropic" }));

		await waitFor(() =>
			expect(router.state.location.search).toContain("provider=prov-anthropic"),
		);

		await user.click(
			await screen.findByRole("button", { name: /Claude Sonnet 4.5/i }),
		);

		await waitFor(() =>
			expect(router.state.location.pathname).toBe(
				"/ai/settings/models/model-claude",
			),
		);
		expect(router.state.location.search).toContain("provider=prov-anthropic");
	});
});
