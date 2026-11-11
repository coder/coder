import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { MockTemplate } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import TemplatesPage from "./TemplatesPage";

describe("TemplatesPage", () => {
	it("opens a template from its name link with the keyboard", async () => {
		const user = userEvent.setup();
		const { router } = renderWithAuth(<TemplatesPage />, {
			route: "/templates",
			path: "/templates",
		});

		const link = await screen.findByRole("link", {
			name: MockTemplate.display_name,
		});
		link.focus();
		await user.keyboard("{Enter}");

		expect(router.state.location.pathname).toMatch(
			new RegExp(`/templates/(.+/)?${MockTemplate.name}$`),
		);
	});

	it("resets a filter that matches no templates", async () => {
		const user = userEvent.setup();
		server.use(
			http.get("/api/v2/templates", ({ request }) => {
				return HttpResponse.json(
					new URL(request.url).searchParams.get("q") ? [] : [MockTemplate],
				);
			}),
		);
		const { router } = renderWithAuth(<TemplatesPage />, {
			path: "/templates",
			route: "/templates?filter=searchnotfound",
		});

		await user.click(
			await screen.findByRole("button", { name: "Reset filters" }),
		);

		await waitFor(() =>
			expect(
				new URLSearchParams(router.state.location.search).get("filter"),
			).toBe(""),
		);
	});
});
