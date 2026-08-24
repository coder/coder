import { render, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { expect, it } from "vitest";
import { ModelDefaultsRedirect } from "./ModelDefaultsRedirect";

it("redirects to Coder Agents and preserves the query string", async () => {
	const router = createMemoryRouter(
		[
			{
				path: "/ai/settings/models/defaults",
				element: <ModelDefaultsRedirect />,
			},
			{
				path: "/ai/settings/coder-agents",
				element: <div>Coder Agents</div>,
			},
		],
		{
			initialEntries: [
				"/ai/settings/models/defaults?org=engineering&source=bookmark",
			],
		},
	);

	render(<RouterProvider router={router} />);

	await waitFor(() => {
		expect(router.state.location.pathname).toBe("/ai/settings/coder-agents");
		expect(router.state.location.search).toBe(
			"?org=engineering&source=bookmark",
		);
	});
});
