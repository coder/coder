import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockApiError } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ErrorAlert } from "./ErrorAlert";

describe("ErrorAlert", () => {
	it("copies request details from an API error", async () => {
		const user = userEvent.setup();
		const writeText = vi.spyOn(navigator.clipboard, "writeText");
		const error = mockApiError({
			message: "Failed to create workspace.",
			validations: [{ field: "name", detail: "Name is already taken." }],
			request: { method: "post", url: "/api/v2/workspaces" },
			status: 500,
			statusText: "Internal Server Error",
			headers: {
				"x-coder-request-id": "abc-123",
				date: "Fri, 16 Oct 2026 17:00:00 GMT",
			},
		});
		renderComponent(<ErrorAlert error={error} />);

		await user.click(screen.getByRole("button", { name: "Technical details" }));
		await user.click(
			screen.getByRole("button", { name: "Copy technical details" }),
		);

		expect(writeText).toHaveBeenCalledWith(
			[
				"Request ID: abc-123",
				"Status: 500 Internal Server Error",
				"Request: POST /api/v2/workspaces",
				"Time: Fri, 16 Oct 2026 17:00:00 GMT",
				`Response: ${JSON.stringify(
					{
						validations: [{ field: "name", detail: "Name is already taken." }],
					},
					null,
					2,
				)}`,
			].join("\n"),
		);
	});
});
