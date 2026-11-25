import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockAIProviderOpenAI } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import UpdateProviderPageView from "./UpdateProviderPageView";

describe("UpdateProviderPageView", () => {
	it("opens model settings filtered to the provider", async () => {
		vi.spyOn(API, "getAIProvider").mockResolvedValue(MockAIProviderOpenAI);
		const user = userEvent.setup();
		const { router } = renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/ai/settings/providers/:providerId",
						element: <UpdateProviderPageView />,
					},
					{ path: "/ai/settings/models", element: <div /> },
				],
				{
					initialEntries: [
						`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
					],
				},
			),
		);

		await user.click(
			await screen.findByRole("link", { name: "Model settings" }),
		);

		await waitFor(() =>
			expect(router.state.location.pathname).toBe("/ai/settings/models"),
		);
		expect(
			new URLSearchParams(router.state.location.search).get("provider"),
		).toBe(MockAIProviderOpenAI.id);
	});

	it("keeps the entered API key when the update fails", async () => {
		vi.spyOn(API, "getAIProvider").mockResolvedValue(MockAIProviderOpenAI);
		vi.spyOn(API, "updateAIProvider").mockRejectedValue(new Error("failed"));
		const user = userEvent.setup();
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/ai/settings/providers/:providerId",
						element: <UpdateProviderPageView />,
					},
				],
				{
					initialEntries: [
						`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
					],
				},
			),
		);

		const apiKey = await screen.findByLabelText(/^api key/i);
		await user.type(apiKey, "sk-new");
		await user.click(screen.getByRole("button", { name: "Update provider" }));

		await screen.findByText("failed");
		expect(apiKey).toHaveValue("sk-new");
	});
});
