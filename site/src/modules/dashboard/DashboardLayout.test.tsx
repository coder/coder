import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen } from "@testing-library/react";
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

	expect(skipToContentLink).toHaveAttribute("href", "#main-content");
	expect(
		skipToContentLink.compareDocumentPosition(navigation) &
			Node.DOCUMENT_POSITION_FOLLOWING,
	).toBeTruthy();
});
