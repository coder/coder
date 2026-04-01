import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { DashboardLayout } from "./DashboardLayout";

test("Show the new Coder version notification", async () => {
	server.use(
		http.get("/api/v2/updatecheck", () => {
			return HttpResponse.json({
				current: false,
				version: "v0.12.9",
				url: "https://github.com/coder/coder/releases/tag/v0.12.9",
			});
		}),
	);
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await screen.findByTestId("update-check-snackbar");
});

test("renders a skip link before navigation content", async () => {
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await waitForLoaderToBeRemoved();

	const skipToContentLink = screen.getByRole("link", {
		name: "Skip to main content",
	});
	const navigation = screen.getAllByRole("navigation")[0];
	const mainContent = document.getElementById("main-content");

	expect(skipToContentLink).toHaveAttribute("href", "#main-content");
	expect(mainContent).toHaveAttribute("tabindex", "-1");
	expect(
		skipToContentLink.compareDocumentPosition(navigation) &
			Node.DOCUMENT_POSITION_FOLLOWING,
	).toBeTruthy();
});

test("moves focus to main content when skip link is clicked", async () => {
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await waitForLoaderToBeRemoved();

	const user = userEvent.setup();
	const skipToContentLink = screen.getByRole("link", {
		name: "Skip to main content",
	});
	const mainContent = document.getElementById("main-content");

	expect(mainContent).not.toBeNull();
	await user.click(skipToContentLink);
	expect(mainContent).toHaveFocus();
});
