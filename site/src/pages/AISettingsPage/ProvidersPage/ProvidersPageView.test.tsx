import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { MockAIProviderOpenAI } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import ProvidersPageView from "./ProvidersPageView";

it("opens a cached provider while the list refreshes", async () => {
	const user = userEvent.setup();
	const { router } = renderWithRouter(
		createMemoryRouter(
			[
				{
					path: "/ai/settings/providers",
					element: (
						<ProvidersPageView
							isLoading={false}
							isFetching
							error={null}
							providers={[MockAIProviderOpenAI]}
						/>
					),
				},
				{ path: "/ai/settings/providers/:provider", element: null },
			],
			{ initialEntries: ["/ai/settings/providers"] },
		),
	);

	await user.click(screen.getByRole("button", { name: /OpenAI/ }));
	await waitFor(() =>
		expect(router.state.location.pathname).toBe(
			`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
		),
	);
});
