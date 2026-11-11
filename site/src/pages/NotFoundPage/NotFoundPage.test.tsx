import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import NotFoundPage from "./NotFoundPage";

describe("NotFoundPage", () => {
	it("navigates to the dashboard from an unknown route", async () => {
		const user = userEvent.setup();
		const { router } = renderWithAuth(<NotFoundPage />, {
			path: "/missing",
			route: "/missing",
			extraRoutes: [{ path: "/", element: <div /> }],
		});

		await user.click(
			await screen.findByRole("link", { name: "Go to dashboard" }),
		);
		expect(router.state.location.pathname).toBe("/");
	});
});
