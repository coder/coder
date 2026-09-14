import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { expect, it } from "vitest";
import { MockNoPermissions } from "#/testHelpers/entities";
import AISettingsSidebarView from "./AISettingsSidebarView";

it("links spend viewers to the Spend page", async () => {
	const user = userEvent.setup();
	const router = createMemoryRouter(
		[
			{
				path: "/ai/settings",
				element: (
					<AISettingsSidebarView
						permissions={MockNoPermissions}
						canViewAISpend
					/>
				),
			},
			{ path: "/ai/settings/spend", element: <div /> },
		],
		{ initialEntries: ["/ai/settings"] },
	);
	render(<RouterProvider router={router} />);
	const link = screen.getByRole("link", { name: "Spend" });
	expect(link).toHaveAttribute("href", "/ai/settings/spend");
	await user.click(link);
	expect(router.state.location.pathname).toBe("/ai/settings/spend");
});
