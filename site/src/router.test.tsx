import { render, screen } from "@testing-library/react";
import { createMemoryRouter, matchRoutes, RouterProvider } from "react-router";
import { expect, it } from "vitest";
import { router } from "./router";

it("redirects the old personal skills settings path to the skills page", async () => {
	const oldPath = "/agents/settings/personal-skills";
	const match = matchRoutes(router.routes, oldPath)?.at(-1);
	const memoryRouter = createMemoryRouter(
		[
			{ path: oldPath, element: match?.route.element },
			{ path: "/agents/settings/skills", element: <div>Skills</div> },
		],
		{ initialEntries: [oldPath] },
	);

	render(<RouterProvider router={memoryRouter} />);

	await screen.findByText("Skills");
	expect(memoryRouter.state.location.pathname).toBe("/agents/settings/skills");
});
