import { render, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { expect, it } from "vitest";
import { UserSettingsIndexRedirect } from "./Layout";

it("redirects the settings index to account", async () => {
	const router = createMemoryRouter(
		[
			{ path: "/settings", element: <UserSettingsIndexRedirect /> },
			{ path: "/settings/account", element: <div>Account</div> },
		],
		{ initialEntries: ["/settings"] },
	);

	render(<RouterProvider router={router} />);

	await waitFor(() => {
		expect(router.state.location.pathname).toBe("/settings/account");
	});
});
