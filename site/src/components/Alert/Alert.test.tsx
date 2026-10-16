import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { Alert, AlertDescription, AlertTitle } from "./Alert";

describe("AlertTitle", () => {
	it("renders as an h2 heading", () => {
		render(
			<Alert>
				<AlertTitle>Deployment warning</AlertTitle>
				<AlertDescription>Something needs your attention.</AlertDescription>
			</Alert>,
		);

		expect(
			screen.getByRole("heading", { level: 2, name: "Deployment warning" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("heading", { level: 1, name: "Deployment warning" }),
		).not.toBeInTheDocument();
	});
});

describe("Alert", () => {
	it("calls onDismiss when dismissed", async () => {
		const user = userEvent.setup();
		const onDismiss = vi.fn();
		render(
			<Alert dismissible onDismiss={onDismiss}>
				Dismiss me
			</Alert>,
		);

		await user.click(screen.getByRole("button", { name: "Dismiss" }));

		expect(onDismiss).toHaveBeenCalledTimes(1);
	});

	it("copies technical details", async () => {
		const user = userEvent.setup();
		const writeText = vi.spyOn(navigator.clipboard, "writeText");
		renderComponent(
			<Alert
				severity="error"
				details={[
					{ label: "Request ID", value: "abc-123" },
					{ label: "Status", value: "500 Internal Server Error" },
				]}
			>
				Request failed
			</Alert>,
		);

		await user.click(screen.getByRole("button", { name: "Technical details" }));
		await user.click(
			screen.getByRole("button", { name: "Copy technical details" }),
		);

		expect(writeText).toHaveBeenCalledWith(
			"Request ID: abc-123\nStatus: 500 Internal Server Error",
		);
	});
});
