import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { getModelSelectorHelp } from "./ModelSelectorHelp";

describe("getModelSelectorHelp", () => {
	it("returns undefined while the model catalog is loading", () => {
		expect(
			getModelSelectorHelp({
				isModelCatalogLoading: true,
				hasModelOptions: false,
				hasConfiguredModels: true,
				hasUserFixableModelProviders: true,
			}),
		).toBeUndefined();
	});

	it("returns undefined when model options are available", () => {
		expect(
			getModelSelectorHelp({
				isModelCatalogLoading: false,
				hasModelOptions: true,
				hasConfiguredModels: true,
				hasUserFixableModelProviders: true,
			}),
		).toBeUndefined();
	});

	it("returns undefined when no models are configured", () => {
		expect(
			getModelSelectorHelp({
				isModelCatalogLoading: false,
				hasModelOptions: false,
				hasConfiguredModels: false,
				hasUserFixableModelProviders: true,
			}),
		).toBeUndefined();
	});

	it("returns settings help when configured models are user-fixable", () => {
		render(
			<MemoryRouter>
				{getModelSelectorHelp({
					isModelCatalogLoading: false,
					hasModelOptions: false,
					hasConfiguredModels: true,
					hasUserFixableModelProviders: true,
				})}
			</MemoryRouter>,
		);

		expect(document.body).toHaveTextContent(
			"Configure your API keys in Settings to enable models.",
		);
		expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute(
			"href",
			"/agents/settings/api-keys",
		);
	});

	it("returns undefined when configured models are not user-fixable", () => {
		expect(
			getModelSelectorHelp({
				isModelCatalogLoading: false,
				hasModelOptions: false,
				hasConfiguredModels: true,
				hasUserFixableModelProviders: false,
			}),
		).toBeUndefined();
	});
});
