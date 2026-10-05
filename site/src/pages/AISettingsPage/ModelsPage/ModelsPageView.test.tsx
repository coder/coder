import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import {
	MockDefaultOrganization,
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

const renderModelsPageView = () => {
	const element = (
		<OrganizationModelsContext.Provider
			value={{
				organization: MockDefaultOrganization,
				accessibleOrganizations: [MockDefaultOrganization],
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
			{ initialEntries: ["/ai/settings/models"] },
		),
	);
};

describe("ModelsPageView", () => {
	it("keeps the provider filter in the URL when opening a model", async () => {
		const user = userEvent.setup();
		const { router } = renderModelsPageView();

		await user.click(
			screen.getByRole("combobox", { name: /filter by provider/i }),
		);
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

	it("selects an add-model provider by its raw provider type", async () => {
		const user = userEvent.setup();
		const { router } = renderModelsPageView();

		await user.click(screen.getByRole("button", { name: /add model/i }));
		// The display label "AWS Bedrock" is capitalized, so a lowercase match
		// at the end of the name only succeeds when the raw type is rendered.
		await user.click(
			await screen.findByRole("menuitem", { name: /AWS Bedrock\s*bedrock$/ }),
		);

		await waitFor(() =>
			expect(router.state.location.search).toContain("provider=prov-bedrock"),
		);
	});
});
